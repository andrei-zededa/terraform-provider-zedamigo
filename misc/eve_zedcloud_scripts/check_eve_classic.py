#!/usr/bin/env python3
"""Check classic EVE-OS connectivity for a node + VM app-instance.

Verifies that every VM interface visible inside the guest matches
the host-side plumbing (TAP -> host bridge -> physical NIC) and
what Zedcloud reports. Degrades gracefully if SSH to the VM is not
reachable -- still checks the node and Zedcloud.
"""
from __future__ import annotations

import json
import re
import re as _re
import sys
from typing import Optional

import _eve_check_common as common
from _eve_check_common import (
    AddrInfo, LinkInfo, NiInfo, ZcIface,
    build_argparser, finalize_config,
    ssh_node, ssh_vm, zedcloud_get,
    parse_ip_d_link, parse_ip_addr,
    zc_node_interfaces, zc_app_interfaces, find_app_uuid,
    fetch_app_interfaces, fetch_ni, guess_ni_kind_from_bridge,
    macs_equal, ip4_only,
    section, pass_, fail_, warn_, info_,
)


def main() -> int:
    ap = build_argparser(__doc__ or "")
    args = ap.parse_args()
    finalize_config(args)

    n_pass = n_warn = n_fail = 0

    def tally(line: str) -> None:
        nonlocal n_pass, n_warn, n_fail
        plain = _re.sub(r"\033\[[0-9;]*m", "", line)
        if plain.startswith("PASS"):
            n_pass += 1
        elif plain.startswith("WARN"):
            n_warn += 1
        elif plain.startswith("FAIL"):
            n_fail += 1
        print(line)

    # --- 1. NODE: SSH and identify --------------------------------------
    print(section("Node — SSH and identify"))
    r = ssh_node(args, "eve uuid; eve version; eve hv; eve app list")
    if not r.ok:
        tally(fail_(f"SSH to EVE node (port {args.node_port}) failed: {r.error}"))
        return _finish(n_pass, n_warn, n_fail)
    lines = r.stdout.splitlines()
    node_uuid = (args.node_uuid or (lines[0].strip() if lines else "")).strip()
    eve_version = lines[1].strip() if len(lines) > 1 else ""
    hv = lines[2].strip() if len(lines) > 2 else ""
    tally(pass_(f"node SSH OK — uuid={node_uuid} eve={eve_version} hv={hv}"))
    if hv and hv != "kvm":
        tally(warn_(f"node reports hv={hv}; this script is intended for classic kvm flavor"))

    # find app uuid via `eve app list` parsing if not provided
    app_uuid = args.app_uuid
    if not app_uuid:
        for line in lines:
            m = line.strip().split()
            # row format: "<displayname> <uuid> VM(HVM) <status>"
            if len(m) >= 4 and m[2].startswith("VM") and "-" in m[1]:
                app_uuid = m[1]
                break
    if app_uuid:
        tally(info_(f"app-instance uuid: {app_uuid}"))
    else:
        tally(warn_("could not auto-discover app-instance uuid from `eve app list`"))

    # --- 2. NODE: host networking ---------------------------------------
    print(section("Node — host-side networking"))
    r_link = ssh_node(args, "ip -d link show")
    r_addr = ssh_node(args, "ip addr show")
    if not r_link.ok or not r_addr.ok:
        tally(fail_(f"could not read ip link/addr on node: {r_link.error or r_addr.error}"))
        return _finish(n_pass, n_warn, n_fail)
    node_links = parse_ip_d_link(r_link.stdout)
    node_addrs = parse_ip_addr(r_addr.stdout)
    by_name = {li.base: li for li in node_links}
    nbus = [li for li in node_links if li.base.startswith("nbu") and "x1" in li.base]
    tally(info_(f"found {len(nbus)} app NIC host handles: " + ", ".join(li.base for li in nbus)))

    # for each nbu*x1: must be type tun-tap, must be bridge_slave, master is a bridge
    for li in nbus:
        if li.is_tap:
            tally(pass_(f"{li.base}: TAP device (classic flavor) — as expected"))
        elif li.kind == "veth":
            tally(fail_(f"{li.base}: veth — looks like EVE-K, not classic"))
        else:
            tally(warn_(f"{li.base}: kind={li.kind!r} (expected tun/tap)"))
        if li.master:
            br = by_name.get(li.master)
            if br and br.kind == "bridge":
                tally(pass_(f"{li.base}: slave of bridge {li.master} (mac {br.mac})"))
            elif br:
                tally(warn_(f"{li.base}: master {li.master} is not a bridge (kind={br.kind})"))
            else:
                tally(warn_(f"{li.base}: master {li.master} not found in link list"))
        else:
            tally(fail_(f"{li.base}: has no master bridge"))

    # --- 3. NODE vs Zedcloud --------------------------------------------
    print(section("Node ↔ Zedcloud — uplink IPs/MACs"))
    node_status, err = zedcloud_get(args, f"/api/v1/devices/id/{node_uuid}/status")
    if err:
        tally(fail_(f"Zedcloud /devices/id/{node_uuid}/status: {err}"))
        node_zc = []
    else:
        node_zc = zc_node_interfaces(node_status)
        tally(info_(f"node Zedcloud state: admin={node_status.get('adminState')} "
                    f"run={node_status.get('runState')}"))
        # cross-check each ethN bridge IP with Zedcloud
        for zi in node_zc:
            host_if = node_addrs.get(zi.ifname)
            if not zi.up:
                tally(info_(f"{zi.ifname}: Zedcloud reports down (uplink={zi.up}) — skipping"))
                continue
            if not host_if:
                tally(warn_(f"{zi.ifname}: Zedcloud says up with {zi.ips} but interface not "
                            f"present on the node"))
                continue
            ssh_ips = ip4_only(host_if.ipv4)
            ips_match = all(ip in ssh_ips for ip in zi.ips if "." in ip)
            mac_ok = macs_equal(zi.mac, host_if.mac)
            if mac_ok and ips_match:
                tally(pass_(f"{zi.ifname}: SSH IPs {ssh_ips} mac {host_if.mac} match "
                            f"Zedcloud ({zi.ips}, mac {zi.mac})"))
            else:
                tally(fail_(f"{zi.ifname}: mismatch — SSH ips={ssh_ips} mac={host_if.mac} "
                            f"vs Zedcloud ips={zi.ips} mac={zi.mac}"))

    # --- 4. App-instance: Zedcloud --------------------------------------
    print(section("App-instance ↔ Zedcloud"))
    app_status = None
    app_zc: list[ZcIface] = []
    ni_by_iface: dict[str, NiInfo] = {}
    if not app_uuid:
        app_uuid = find_app_uuid(node_status or {})
        if app_uuid:
            tally(info_(f"app-instance uuid discovered from node status: {app_uuid}"))
    if app_uuid:
        app_status, err = zedcloud_get(args, f"/api/v1/apps/instances/id/{app_uuid}/status")
        if err:
            tally(fail_(f"Zedcloud /apps/instances/id/{app_uuid}/status: {err}"))
        else:
            app_zc = zc_app_interfaces(app_status)
            tally(info_(f"app Zedcloud state: admin={app_status.get('adminState')} "
                        f"run={app_status.get('runState')}"))
            for zi in app_zc:
                tally(info_(f"  {zi.ifname}: mac={zi.mac} ips={zi.ips} gw={zi.gws}"))
        # also fetch the app config to learn which NI each interface targets
        ifaces, err = fetch_app_interfaces(args, app_uuid)
        if err:
            tally(warn_(f"could not fetch app config: {err} (NI kinds will use bridge-name fallback)"))
        else:
            ni_cache: dict = {}
            for cfg_if in ifaces:
                niid = cfg_if.get("netinstid")
                intfname = cfg_if.get("intfname") or ""
                if not niid:
                    continue
                ni, nerr = fetch_ni(args, niid, ni_cache)
                if ni:
                    ni_by_iface[intfname] = ni
                    tally(info_(f"  {intfname} → NI {ni.name} [{ni.colorized()}] port={ni.port}"))
                elif nerr:
                    tally(warn_(f"  {intfname}: NI {niid} fetch failed: {nerr}"))
    else:
        tally(warn_("no app-instance uuid — skipping Zedcloud app status"))

    # --- 5. VM (guest) --------------------------------------------------
    print(section("App-instance — guest-side networking"))
    vm_addr = ssh_vm(args, "hostname; ip addr show; echo ===ROUTES===; ip -4 route")
    vm_addrs: dict = {}
    vm_reachable = vm_addr.ok
    if not vm_reachable:
        tally(warn_(f"SSH to VM (port {args.vm_port}) not available: {vm_addr.error} "
                    "— continuing with node+Zedcloud only"))
    else:
        head, _, rest = vm_addr.stdout.partition("===ROUTES===")
        vm_addrs = parse_ip_addr(head)
        tally(pass_(f"VM SSH OK ({len(vm_addrs)} interfaces)"))
        for name, ai in vm_addrs.items():
            if name == "lo":
                continue
            tally(info_(f"  {name}: mac={ai.mac} ipv4={ai.ipv4}"))

    # --- 6. Cross-check VM NICs vs host TAPs vs Zedcloud app ------------
    print(section("Per-interface cross-check (VM ↔ host TAP ↔ host bridge ↔ Zedcloud)"))
    # Match by MAC: each VM NIC mac should match exactly one Zedcloud app NIC.
    by_mac_vm = {ai.mac.lower(): ai for ai in vm_addrs.values()
                 if ai.mac and ai.mac != "00:00:00:00:00:00"}
    if not app_zc:
        tally(warn_("no Zedcloud app data — cannot correlate VM NICs against controller"))
        return _finish(n_pass, n_warn, n_fail)
    for zi in app_zc:
        guest_ai = by_mac_vm.get((zi.mac or "").lower()) if zi.mac else None
        # find the matching nbu*x1 on host: by index (app_eth0 -> nbu1x1, eth1 -> nbu2x1, eth2 -> nbu3x1)
        idx = _eth_index(zi.ifname)
        host_handle = next((li for li in nbus if li.base == f"nbu{idx}x1"), None) if idx else None
        bridge_name = host_handle.master if host_handle else None
        bridge = by_name.get(bridge_name) if bridge_name else None
        # NI kind (from Zedcloud, else heuristic on bridge name)
        ni = ni_by_iface.get(zi.ifname)
        if ni:
            ni_label = f"{ni.colorized()} ({ni.name})"
            ni_kind_value = ni.kind
        else:
            guessed = guess_ni_kind_from_bridge(bridge.name if bridge else None)
            if guessed:
                short = "LOCAL/NAT" if guessed.endswith("_LOCAL") else "SWITCH (L2)"
                ni_label = f"{short} (guessed from bridge)"
                ni_kind_value = guessed
            else:
                ni_label = "?"
                ni_kind_value = ""
        # report
        bits = [f"{common.BOLD}{zi.ifname}{common.RESET}"]
        bits.append(f"NI={ni_label}")
        bits.append(f"mac={zi.mac}")
        bits.append(f"zc_ips={zi.ips}")
        bits.append(f"zc_gw={zi.gws or ['-']}")
        if host_handle:
            bits.append(f"host_handle={host_handle.base}({'TAP' if host_handle.is_tap else host_handle.kind})")
        else:
            bits.append("host_handle=MISSING")
        if bridge:
            bits.append(f"bridge={bridge.name}({bridge.mac})")
        print("    " + "  ".join(bits))
        # sanity check NI kind vs host plumbing
        if ni_kind_value.endswith("_LOCAL") and bridge and not bridge.name.startswith("bn"):
            tally(warn_(f"{zi.ifname}: NI kind LOCAL but bridge {bridge.name} is not a bnN"))
        if ni_kind_value.endswith("_SWITCH") and bridge and not re.match(r"^eth\d+$", bridge.name):
            tally(warn_(f"{zi.ifname}: NI kind SWITCH but bridge {bridge.name} is not an ethN"))
        # checks
        if host_handle and host_handle.is_tap:
            tally(pass_(f"{zi.ifname}: host handle is a TAP (classic) on bridge {bridge_name}"))
        elif host_handle:
            tally(fail_(f"{zi.ifname}: host handle {host_handle.base} is not a TAP "
                        f"(kind={host_handle.kind})"))
        else:
            tally(fail_(f"{zi.ifname}: no nbu{idx}x1 found on host"))
        if vm_reachable:
            if guest_ai:
                if all(ip in ip4_only(guest_ai.ipv4) for ip in zi.ips if "." in ip):
                    tally(pass_(f"{zi.ifname}: guest IPs {ip4_only(guest_ai.ipv4)} include "
                                f"Zedcloud-reported {zi.ips}"))
                else:
                    tally(fail_(f"{zi.ifname}: guest IPs {ip4_only(guest_ai.ipv4)} do not include "
                                f"Zedcloud {zi.ips}"))
            else:
                tally(warn_(f"{zi.ifname}: no matching VM NIC by MAC {zi.mac}"))

    return _finish(n_pass, n_warn, n_fail)


def _eth_index(zc_ifname: str) -> Optional[int]:
    # app_eth0 -> 1 (NAT NIC is nbu1x1), app_eth1 -> 2, app_eth2 -> 3, ...
    import re as _re
    m = _re.match(r"app_eth(\d+)$", zc_ifname or "")
    if not m:
        return None
    return int(m.group(1)) + 1


def _finish(n_pass: int, n_warn: int, n_fail: int, **_extra) -> int:
    print(section("Summary"))
    print(f"  PASS={n_pass}  WARN={n_warn}  FAIL={n_fail}")
    return 0 if n_fail == 0 else 1


if __name__ == "__main__":
    sys.exit(main())
