#!/usr/bin/env python3
"""Check EVE-K (KubeVirt) connectivity for a node + VM app-instance.

Walks the per-NIC chain
    VMI -> tap<h> -> k6t-<h> -> <h>-nic -> nbu*x1 -> host bridge -> physical NIC
inside the virt-launcher pod's network namespace, plus the usual node + VM
SSH + Zedcloud cross-checks. Degrades gracefully when the VM is not
reachable -- still checks node, pod netns, kubevirt VMI, and Zedcloud.
"""
from __future__ import annotations

import json
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

    # --- 1. Node identify -----------------------------------------------
    print(section("Node — SSH and identify"))
    r = ssh_node(args, "eve uuid; eve version; eve hv")
    if not r.ok:
        tally(fail_(f"SSH to EVE node (port {args.node_port}) failed: {r.error}"))
        return _finish(n_pass, n_warn, n_fail)
    lines = r.stdout.splitlines()
    node_uuid = (args.node_uuid or (lines[0].strip() if lines else "")).strip()
    eve_version = lines[1].strip() if len(lines) > 1 else ""
    hv = lines[2].strip() if len(lines) > 2 else ""
    tally(pass_(f"node SSH OK — uuid={node_uuid} eve={eve_version} hv={hv}"))
    if "k" not in eve_version and hv != "k":
        tally(warn_(f"eve version {eve_version!r} / hv {hv!r} does not look like an EVE-K build"))

    # --- 2. Host networking ---------------------------------------------
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
    pod_netns: Optional[str] = None
    for li in nbus:
        if li.kind == "veth":
            tally(pass_(f"{li.base}: veth (EVE-K flavor)"))
        elif li.is_tap:
            tally(fail_(f"{li.base}: TAP — looks like classic EVE, not EVE-K"))
        else:
            tally(warn_(f"{li.base}: kind={li.kind!r} (expected veth)"))
        if li.link_netns and pod_netns is None:
            pod_netns = li.link_netns
        if li.master:
            br = by_name.get(li.master)
            if br and br.kind == "bridge":
                tally(pass_(f"{li.base}: slave of host bridge {li.master} (mac {br.mac})"))
            else:
                tally(warn_(f"{li.base}: master {li.master} not a bridge "
                            f"(kind={(br.kind if br else None)!r})"))
        else:
            tally(fail_(f"{li.base}: has no master bridge"))
    if pod_netns:
        tally(info_(f"virt-launcher pod netns: {pod_netns}"))
    else:
        tally(warn_("no link-netns hint found on any nbu*x1 — cannot inspect pod side"))

    # --- 3. Pod netns inspection ----------------------------------------
    pod_links: list[LinkInfo] = []
    if pod_netns:
        print(section("Virt-launcher pod netns — inside-pod stack"))
        r_p = ssh_node(args, f"ip netns exec {pod_netns} ip -d link show")
        if not r_p.ok:
            tally(warn_(f"ip netns exec {pod_netns} failed: {r_p.error}"))
        else:
            pod_links = parse_ip_d_link(r_p.stdout)
            k6t = [li for li in pod_links if li.base.startswith("k6t-")]
            taps = [li for li in pod_links if li.base.startswith("tap") and li.base != "tap0"]
            nics = [li for li in pod_links if li.base.endswith("-nic")]
            pods = [li for li in pod_links if li.base.startswith("pod") and len(li.base) > 3]
            tally(info_(f"pod has {len(k6t)} k6t-* bridges, {len(taps)} tap*, "
                        f"{len(nics)} *-nic veths, {len(pods)} pod* dummies"))
            # Expect each VMI NIC to have a (k6t, tap, *-nic, pod*) quad sharing a hash.
            quads: dict[str, dict[str, LinkInfo]] = {}
            for li in pod_links:
                m = _re.match(r"(?:k6t-|tap|pod)([0-9a-f]+)$", li.base)
                if m:
                    h = m.group(1)
                    quads.setdefault(h, {})[li.base.replace(h, "<h>")] = li
                    continue
                m = _re.match(r"([0-9a-f]+)-nic$", li.base)
                if m:
                    h = m.group(1)
                    quads.setdefault(h, {})["<h>-nic"] = li
            for h, q in quads.items():
                missing = [n for n in ("k6t-<h>", "tap<h>", "<h>-nic", "pod<h>") if n not in q]
                if missing:
                    tally(warn_(f"pod NIC hash={h}: missing {missing}"))
                else:
                    tally(pass_(f"pod NIC hash={h}: k6t bridge + tap + veth + pod dummy all present"))
                # check that tap and *-nic are slaves of the k6t bridge
                br = q.get("k6t-<h>")
                tap = q.get("tap<h>")
                nic = q.get("<h>-nic")
                if br and tap and tap.master == br.base:
                    tally(pass_(f"  tap{h} is bridge_slave of {br.base}"))
                elif br and tap:
                    tally(fail_(f"  tap{h} master={tap.master} (expected {br.base})"))
                if br and nic and nic.master == br.base:
                    tally(pass_(f"  {h}-nic is bridge_slave of {br.base}"))
                elif br and nic:
                    tally(fail_(f"  {h}-nic master={nic.master} (expected {br.base})"))

    # --- 4. Node vs Zedcloud --------------------------------------------
    print(section("Node ↔ Zedcloud — uplink IPs/MACs"))
    node_status, err = zedcloud_get(args, f"/api/v1/devices/id/{node_uuid}/status")
    node_zc: list[ZcIface] = []
    if err:
        tally(fail_(f"Zedcloud /devices/id/{node_uuid}/status: {err}"))
    else:
        node_zc = zc_node_interfaces(node_status)
        tally(info_(f"node Zedcloud state: admin={node_status.get('adminState')} "
                    f"run={node_status.get('runState')}"))
        for zi in node_zc:
            if not zi.up:
                tally(info_(f"{zi.ifname}: Zedcloud reports down — skipping"))
                continue
            host_if = node_addrs.get(zi.ifname)
            if not host_if:
                tally(warn_(f"{zi.ifname}: Zedcloud says up with {zi.ips} but iface absent on node"))
                continue
            ssh_ips = ip4_only(host_if.ipv4)
            ips_match = all(ip in ssh_ips for ip in zi.ips if "." in ip)
            mac_ok = macs_equal(zi.mac, host_if.mac)
            if mac_ok and ips_match:
                tally(pass_(f"{zi.ifname}: SSH IPs {ssh_ips} mac {host_if.mac} match Zedcloud"))
            else:
                tally(fail_(f"{zi.ifname}: mismatch SSH ips={ssh_ips} mac={host_if.mac} "
                            f"vs Zedcloud ips={zi.ips} mac={zi.mac}"))

    # --- 5. KubeVirt VMI summary (via eve exec kube) --------------------
    print(section("KubeVirt — VMI and virt-launcher pod"))
    r_vmi = ssh_node(args, "eve exec kube kubectl get vmi -A "
                           "-o jsonpath='{range .items[*]}{.metadata.namespace}/{.metadata.name}|"
                           "{.status.phase}|{.spec.domain.devices.interfaces}|"
                           "{.status.interfaces}{\"\\n\"}{end}' 2>/dev/null")
    if not r_vmi.ok:
        tally(warn_(f"could not run kubectl via eve exec kube: {r_vmi.error}"))
    else:
        vmi_lines = [ln for ln in r_vmi.stdout.splitlines() if ln.strip()]
        if not vmi_lines:
            tally(warn_("no VMIs found in any namespace — is the app instance scheduled?"))
        for ln in vmi_lines:
            tally(info_(ln))

    # --- 6. App-instance Zedcloud ---------------------------------------
    print(section("App-instance ↔ Zedcloud"))
    app_uuid = args.app_uuid
    if not app_uuid and node_status:
        app_uuid = find_app_uuid(node_status)
        if app_uuid:
            tally(info_(f"app-instance uuid discovered from node status: {app_uuid}"))
    app_status = None
    app_zc: list[ZcIface] = []
    ni_by_iface: dict[str, NiInfo] = {}
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
        tally(warn_("no app-instance uuid known — skipping Zedcloud app status"))

    # --- 7. VM guest side -----------------------------------------------
    print(section("App-instance — guest-side networking"))
    vm_addr = ssh_vm(args, "hostname; ip addr show; echo ===ROUTES===; ip -4 route")
    vm_addrs: dict[str, AddrInfo] = {}
    vm_reachable = vm_addr.ok
    if not vm_reachable:
        tally(warn_(f"SSH to VM (port {args.vm_port}) not available: {vm_addr.error} "
                    "— continuing with node+pod+Zedcloud only"))
    else:
        head, _, _ = vm_addr.stdout.partition("===ROUTES===")
        vm_addrs = parse_ip_addr(head)
        tally(pass_(f"VM SSH OK ({len(vm_addrs)} interfaces)"))
        for name, ai in vm_addrs.items():
            if name == "lo":
                continue
            tally(info_(f"  {name}: mac={ai.mac} ipv4={ai.ipv4}"))

    # --- 8. Per-interface end-to-end correlation ------------------------
    print(section("Per-interface cross-check"))
    if not app_zc:
        tally(warn_("no Zedcloud app data — cannot correlate VM NICs end-to-end"))
        return _finish(n_pass, n_warn, n_fail)

    by_mac_vm = {ai.mac.lower(): ai for ai in vm_addrs.values()
                 if ai.mac and ai.mac != "00:00:00:00:00:00"}
    # find pod-side veths so we can match by ifindex peering
    pod_veths_by_mac = {li.mac.lower(): li for li in pod_links if li.mac and li.kind == "veth"}

    for zi in app_zc:
        idx = _eth_index(zi.ifname)
        host_handle = next((li for li in nbus if li.base == f"nbu{idx}x1"), None) if idx else None
        bridge = by_name.get(host_handle.master) if host_handle and host_handle.master else None
        # peer veth on pod side — host_handle.peer_ifindex links to pod-side ifindex
        peer = None
        if host_handle and host_handle.peer_ifindex is not None:
            peer = next((li for li in pod_links if li.idx == host_handle.peer_ifindex), None)
        # k6t bridge: the peer's master
        k6t = None
        tap = None
        if peer:
            k6t = next((li for li in pod_links if li.base == peer.master), None) if peer.master else None
            if k6t:
                tap = next((li for li in pod_links
                            if li.base.startswith("tap") and li.master == k6t.base), None)
        guest_ai = by_mac_vm.get((zi.mac or "").lower()) if zi.mac else None

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
        print(f"\n  {common.BOLD}{zi.ifname}{common.RESET}  NI={ni_label}  mac={zi.mac}  zc_ips={zi.ips}  zc_gw={zi.gws or ['-']}")
        if ni_kind_value.endswith("_LOCAL") and bridge and not bridge.name.startswith("bn"):
            tally(warn_(f"{zi.ifname}: NI kind LOCAL but bridge {bridge.name} is not a bnN"))
        if ni_kind_value.endswith("_SWITCH") and bridge and not _re.match(r"^eth\d+$", bridge.name):
            tally(warn_(f"{zi.ifname}: NI kind SWITCH but bridge {bridge.name} is not an ethN"))
        chain = []
        if guest_ai:
            chain.append(f"guest:{guest_ai.name}")
        else:
            chain.append("guest:?")
        if tap:
            chain.append(f"tap:{tap.base}")
        if k6t:
            chain.append(f"k6t:{k6t.base}")
        if peer:
            chain.append(f"pod-nic:{peer.base}")
        if host_handle:
            chain.append(f"host:{host_handle.base}({'TAP' if host_handle.is_tap else host_handle.kind})")
        if bridge:
            chain.append(f"bridge:{bridge.name}")
        print("    chain: " + " → ".join(chain))

        # checks
        if host_handle and host_handle.kind == "veth":
            tally(pass_(f"{zi.ifname}: host handle is a veth (EVE-K)"))
        elif host_handle:
            tally(fail_(f"{zi.ifname}: host handle {host_handle.base} kind={host_handle.kind}"))
        else:
            tally(fail_(f"{zi.ifname}: no nbu{idx}x1 found on host"))
        if peer and k6t and tap:
            tally(pass_(f"{zi.ifname}: pod-side veth→k6t bridge→tap chain present "
                        f"({peer.base} → {k6t.base} → {tap.base})"))
        elif peer and k6t:
            tally(warn_(f"{zi.ifname}: pod has bridge {k6t.base} but no tap slave"))
        elif peer:
            tally(warn_(f"{zi.ifname}: pod-side veth {peer.base} has no k6t bridge master"))
        else:
            tally(warn_(f"{zi.ifname}: could not find pod-side veth peer "
                        f"(host peer_ifindex={host_handle.peer_ifindex if host_handle else None})"))
        if vm_reachable:
            if guest_ai:
                if all(ip in ip4_only(guest_ai.ipv4) for ip in zi.ips if "." in ip):
                    tally(pass_(f"{zi.ifname}: guest IPs {ip4_only(guest_ai.ipv4)} include Zedcloud {zi.ips}"))
                else:
                    tally(fail_(f"{zi.ifname}: guest IPs {ip4_only(guest_ai.ipv4)} do not include Zedcloud {zi.ips}"))
            else:
                tally(warn_(f"{zi.ifname}: no matching VM NIC by MAC {zi.mac}"))

    return _finish(n_pass, n_warn, n_fail)


def _eth_index(zc_ifname: str) -> Optional[int]:
    m = _re.match(r"app_eth(\d+)$", zc_ifname or "")
    if not m:
        return None
    return int(m.group(1)) + 1


def _finish(n_pass: int, n_warn: int, n_fail: int) -> int:
    print(section("Summary"))
    print(f"  PASS={n_pass}  WARN={n_warn}  FAIL={n_fail}")
    return 0 if n_fail == 0 else 1


if __name__ == "__main__":
    sys.exit(main())
