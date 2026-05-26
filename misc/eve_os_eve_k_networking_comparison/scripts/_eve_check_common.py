"""Shared helpers for the EVE connectivity checkers."""
from __future__ import annotations

import argparse
import json
import os
import re
import shlex
import ssl
import subprocess
import sys
import urllib.request
from dataclasses import dataclass, field
from typing import Optional

GREEN = "\033[32m"
RED = "\033[31m"
YELLOW = "\033[33m"
BLUE = "\033[36m"
MAGENTA = "\033[35m"
RESET = "\033[0m"
DIM = "\033[2m"
BOLD = "\033[1m"


def colorize(use: bool):
    if use:
        return
    global GREEN, RED, YELLOW, BLUE, MAGENTA, RESET, DIM, BOLD
    GREEN = RED = YELLOW = BLUE = MAGENTA = RESET = DIM = BOLD = ""


def pass_(msg: str) -> str:
    return f"{GREEN}PASS{RESET}  {msg}"


def fail_(msg: str) -> str:
    return f"{RED}FAIL{RESET}  {msg}"


def warn_(msg: str) -> str:
    return f"{YELLOW}WARN{RESET}  {msg}"


def info_(msg: str) -> str:
    return f"{BLUE}INFO{RESET}  {msg}"


def section(title: str) -> str:
    bar = "=" * len(title)
    return f"\n{BOLD}{title}{RESET}\n{bar}"


# ---------------------------------------------------------------------------
# CLI / config
# ---------------------------------------------------------------------------

def build_argparser(prog_desc: str) -> argparse.ArgumentParser:
    ap = argparse.ArgumentParser(description=prog_desc)
    ap.add_argument("--host", default=os.environ.get("HOST_ADDR"),
                    help="Optional jump host (ProxyCommand). If omitted, --node-host "
                         "(and --vm-host) must be directly reachable.")
    ap.add_argument("--host-user", default=os.environ.get("HOST_USER", "ubnt"),
                    help="Jump-host SSH user (only used when --host is set)")
    ap.add_argument("--host-key", default=os.environ.get("HOST_AUTH_SSH_KEY"),
                    help="SSH private key for the jump host (only used when --host is set)")
    ap.add_argument("--node-host",
                    default=os.environ.get("NODE_SSH_HOST"),
                    help="EVE node SSH host. Defaults to 'localhost' when --host is set, "
                         "otherwise required.")
    ap.add_argument("--node-port", type=int,
                    default=int(os.environ["NODE_SSH_PORT"]) if os.environ.get("NODE_SSH_PORT") else None,
                    help="SSH port for the EVE node")
    ap.add_argument("--node-user", default=os.environ.get("NODE_SSH_USER", "root"))
    ap.add_argument("--node-key", default=os.environ.get("NODE_SSH_KEY"),
                    help="SSH key for the EVE node (default: --host-key, then any agent key)")
    ap.add_argument("--vm-host",
                    default=os.environ.get("VM_SSH_HOST"),
                    help="VM app-instance SSH host (default: same as --node-host)")
    ap.add_argument("--vm-port", type=int,
                    default=int(os.environ["VM_SSH_PORT"]) if os.environ.get("VM_SSH_PORT") else None,
                    help="VM app-instance SSH port (default: node-port + 1)")
    ap.add_argument("--vm-user", default=os.environ.get("VM_SSH_USER", "labuser"))
    ap.add_argument("--vm-key", default=os.environ.get("VM_SSH_KEY"),
                    help="SSH key for the VM (default: --host-key, then any agent key)")
    ap.add_argument("--node-direct", action="store_true",
                    help="Bypass the jump host when SSHing to the node "
                         "(even if --host is set)")
    ap.add_argument("--vm-direct", action="store_true",
                    help="Bypass the jump host when SSHing to the VM "
                         "(even if --host is set)")
    ap.add_argument("--zedcloud",
                    default=os.environ.get("ZEDEDA_CLOUD") or os.environ.get("ZEDEDA_CLOUD_URL"),
                    help="Zedcloud host (e.g. zedcloud.gmwtus.zededa.net)")
    ap.add_argument("--token",
                    default=os.environ.get("ZEDEDA_TOKEN") or os.environ.get("ZEDEDA_CLOUD_TOKEN"),
                    help="Zedcloud API token")
    ap.add_argument("--node-uuid", help="Override node UUID (otherwise discovered via `eve uuid`)")
    ap.add_argument("--app-uuid", help="Override app-instance UUID (otherwise auto-discovered)")
    ap.add_argument("--no-color", action="store_true")
    ap.add_argument("--ssh-timeout", type=int, default=15)
    return ap


def finalize_config(args: argparse.Namespace) -> None:
    colorize(not args.no_color and sys.stdout.isatty())
    # If a jump host is given, targets that go through it default to
    # localhost (port-forwarded on the jump host).
    if args.host:
        if not args.host_key:
            print(fail_("--host is set but --host-key/HOST_AUTH_SSH_KEY is missing"))
            sys.exit(2)
        if not args.node_host and not args.node_direct:
            args.node_host = "localhost"
    required = [("--node-host", args.node_host),
                ("--node-port", args.node_port),
                ("--zedcloud", args.zedcloud),
                ("--token", args.token)]
    missing = [n for n, v in required if not v]
    if missing:
        print(fail_("Missing required arguments: " + ", ".join(missing)))
        sys.exit(2)
    if not args.vm_host:
        args.vm_host = args.node_host
    if args.vm_port is None:
        args.vm_port = args.node_port + 1
    if not args.node_key:
        args.node_key = args.host_key
    if not args.vm_key:
        args.vm_key = args.host_key


# ---------------------------------------------------------------------------
# SSH helpers
# ---------------------------------------------------------------------------

def _proxy_cmd(args, *, direct: bool) -> Optional[str]:
    if direct or not args.host:
        return None
    parts = [
        "ssh", "-i", args.host_key,
        "-o", "StrictHostKeyChecking=accept-new",
        "-o", "UserKnownHostsFile=/dev/null",
        "-o", "LogLevel=ERROR",
        "-W", "%h:%p",
        f"{args.host_user}@{args.host}",
    ]
    return " ".join(shlex.quote(p) for p in parts)


@dataclass
class SshResult:
    ok: bool
    stdout: str = ""
    stderr: str = ""
    rc: int = 0
    error: Optional[str] = None


def _ssh(args, *, host: str, port: int, user: str, key: Optional[str], cmd: str,
         direct: bool = False, timeout: Optional[int] = None) -> SshResult:
    """Run cmd on user@host:port. Uses ProxyCommand via the jump host when
    --host is set and `direct` is False."""
    full = [
        "ssh",
        "-o", "StrictHostKeyChecking=accept-new",
        "-o", "UserKnownHostsFile=/dev/null",
        "-o", "LogLevel=ERROR",
        "-o", "BatchMode=yes",
    ]
    if key:
        full += ["-i", key]
    proxy = _proxy_cmd(args, direct=direct)
    if proxy:
        full += ["-o", f"ProxyCommand={proxy}"]
    full += ["-p", str(port), f"{user}@{host}", cmd]
    try:
        r = subprocess.run(full, capture_output=True, text=True,
                           timeout=timeout or args.ssh_timeout)
    except subprocess.TimeoutExpired:
        return SshResult(ok=False, error="timeout")
    except FileNotFoundError as e:
        return SshResult(ok=False, error=str(e))
    if r.returncode != 0:
        return SshResult(ok=False, stdout=r.stdout, stderr=r.stderr,
                         rc=r.returncode, error=r.stderr.strip() or f"rc={r.returncode}")
    return SshResult(ok=True, stdout=r.stdout, stderr=r.stderr, rc=0)


def ssh_node(args, cmd: str) -> SshResult:
    return _ssh(args, host=args.node_host, port=args.node_port, user=args.node_user,
                key=args.node_key, cmd=cmd, direct=args.node_direct)


def ssh_vm(args, cmd: str) -> SshResult:
    return _ssh(args, host=args.vm_host, port=args.vm_port, user=args.vm_user,
                key=args.vm_key, cmd=cmd, direct=args.vm_direct)


# ---------------------------------------------------------------------------
# Zedcloud helpers
# ---------------------------------------------------------------------------

def zedcloud_get(args, path: str) -> tuple[Optional[dict], Optional[str]]:
    if not args.zedcloud.startswith("http"):
        base = f"https://{args.zedcloud}"
    else:
        base = args.zedcloud
    url = f"{base.rstrip('/')}{path}"
    req = urllib.request.Request(url, headers={
        "Authorization": f"Bearer {args.token}",
        "Accept": "application/json",
    })
    ctx = ssl.create_default_context()
    try:
        with urllib.request.urlopen(req, timeout=20, context=ctx) as resp:
            return json.load(resp), None
    except Exception as e:
        return None, str(e)


# ---------------------------------------------------------------------------
# Parsers for `ip` output
# ---------------------------------------------------------------------------

_LINK_HEADER = re.compile(
    r"^(?P<idx>\d+):\s+(?P<name>[\w@\-.]+):\s+<(?P<flags>[^>]+)>.*?"
    r"(?:master\s+(?P<master>\S+)\s+)?state\s+(?P<state>\S+)"
)


@dataclass
class LinkInfo:
    idx: int
    name: str               # e.g. "nbu2x1@if59" or "eth1"
    base: str               # name with @if... stripped
    peer_ifindex: Optional[int]
    state: str
    flags: list[str]
    master: Optional[str]
    mac: Optional[str]
    kind: Optional[str]     # "bridge", "veth", "tun", "dummy", ...
    is_tap: bool            # tun type tap
    bridge_slave: bool
    link_netns: Optional[str]
    raw: str

    def short(self) -> str:
        bits = [self.name]
        if self.kind:
            bits.append(self.kind)
        if self.is_tap:
            bits.append("(TAP)")
        if self.master:
            bits.append(f"master={self.master}")
        if self.mac:
            bits.append(self.mac)
        if self.link_netns:
            bits.append(f"netns={self.link_netns}")
        return " ".join(bits)


def parse_ip_d_link(text: str) -> list[LinkInfo]:
    """Parse output of `ip -d link show`. Each link is multi-line."""
    out: list[LinkInfo] = []
    blocks: list[list[str]] = []
    cur: list[str] = []
    for line in text.splitlines():
        if re.match(r"^\d+:\s", line):
            if cur:
                blocks.append(cur)
            cur = [line]
        else:
            if cur:
                cur.append(line)
    if cur:
        blocks.append(cur)
    for blk in blocks:
        header = blk[0]
        m = _LINK_HEADER.match(header)
        if not m:
            continue
        name = m.group("name")
        base, _, peer_part = name.partition("@")
        peer_ifindex = None
        if peer_part and peer_part.startswith("if"):
            try:
                peer_ifindex = int(peer_part[2:])
            except ValueError:
                peer_ifindex = None
        flags = [f.strip() for f in m.group("flags").split(",") if f.strip()]
        master = m.group("master")
        state = m.group("state")
        full = "\n".join(blk)
        mac = None
        macm = re.search(r"link/ether\s+([0-9a-f:]{17})", full)
        if macm:
            mac = macm.group(1)
        kind = None
        for k in ("bridge", "veth", "dummy", "tun", "vxlan", "vlan", "bond", "macvlan"):
            if re.search(rf"\b{k}\b", full):
                kind = k
                break
        is_tap = bool(re.search(r"\btun\s+type\s+tap\b", full))
        bridge_slave = "bridge_slave" in full
        link_netns = None
        nm = re.search(r"link-netns\s+(\S+)", full)
        if nm:
            link_netns = nm.group(1)
        out.append(LinkInfo(
            idx=int(m.group("idx")), name=name, base=base,
            peer_ifindex=peer_ifindex, state=state, flags=flags,
            master=master, mac=mac, kind=kind, is_tap=is_tap,
            bridge_slave=bridge_slave, link_netns=link_netns, raw=full,
        ))
    return out


_ADDR_HEADER = re.compile(r"^\d+:\s+(?P<name>[\w@\-.]+):\s+")


@dataclass
class AddrInfo:
    name: str
    base: str
    mac: Optional[str]
    ipv4: list[str] = field(default_factory=list)
    ipv6: list[str] = field(default_factory=list)


def parse_ip_addr(text: str) -> dict[str, AddrInfo]:
    res: dict[str, AddrInfo] = {}
    cur: Optional[AddrInfo] = None
    for line in text.splitlines():
        m = _ADDR_HEADER.match(line)
        if m:
            name = m.group("name")
            base = name.split("@", 1)[0]
            cur = AddrInfo(name=name, base=base, mac=None)
            res[base] = cur
            continue
        if cur is None:
            continue
        m = re.search(r"link/(?:ether|loopback)\s+([0-9a-f:]{17})", line)
        if m:
            cur.mac = m.group(1)
            continue
        m = re.search(r"\binet\s+(\d+\.\d+\.\d+\.\d+/\d+)", line)
        if m:
            cur.ipv4.append(m.group(1))
            continue
        m = re.search(r"\binet6\s+([0-9a-f:]+/\d+)", line)
        if m:
            cur.ipv6.append(m.group(1))
            continue
    return res


# ---------------------------------------------------------------------------
# Zedcloud helpers (status extraction)
# ---------------------------------------------------------------------------

@dataclass
class ZcIface:
    ifname: str
    mac: Optional[str]
    ips: list[str]
    gws: list[str]
    up: bool


def zc_node_interfaces(node_status: dict) -> list[ZcIface]:
    out = []
    for n in node_status.get("netStatusList", []) or []:
        mac = n.get("macAddr") or None
        out.append(ZcIface(
            ifname=n.get("ifName", "?"),
            mac=mac.lower() if mac else None,
            ips=[ip for ip in (n.get("ipAddrs") or []) if not ip.startswith("fe80")],
            gws=[g for g in (n.get("defaultRouters") or []) if g and g != "<nil>"],
            up=bool(n.get("up", False)),
        ))
    return out


def zc_app_interfaces(app_status: dict) -> list[ZcIface]:
    out = []
    for n in app_status.get("netStatusList", []) or []:
        mac = n.get("macAddr") or None
        out.append(ZcIface(
            ifname=n.get("ifName", "?"),
            mac=mac.lower() if mac else None,
            ips=[ip for ip in (n.get("ipAddrs") or []) if not ip.startswith("fe80")],
            gws=[g for g in (n.get("defaultRouters") or []) if g and g != "<nil>"],
            up=bool(n.get("up", True)),
        ))
    return out


@dataclass
class NiInfo:
    uuid: str
    name: str
    kind: str            # e.g. "NETWORK_INSTANCE_KIND_LOCAL" / "_SWITCH"
    port: Optional[str]  # uplink port name as configured

    @property
    def short_kind(self) -> str:
        if self.kind.endswith("_LOCAL"):
            return "LOCAL/NAT"
        if self.kind.endswith("_SWITCH"):
            return "SWITCH (L2)"
        return self.kind or "?"

    def colorized(self) -> str:
        sk = self.short_kind
        if sk == "LOCAL/NAT":
            return f"{MAGENTA}{sk}{RESET}"
        if sk == "SWITCH (L2)":
            return f"{BLUE}{sk}{RESET}"
        return sk


def fetch_app_interfaces(args, app_uuid: str) -> tuple[list[dict], Optional[str]]:
    """Fetch the app instance config and return the `interfaces` list."""
    cfg, err = zedcloud_get(args, f"/api/v1/apps/instances/id/{app_uuid}")
    if err:
        return [], err
    return cfg.get("interfaces") or [], None


def fetch_ni(args, ni_uuid: str, cache: dict) -> tuple[Optional[NiInfo], Optional[str]]:
    """Fetch a network-instance object; results are cached in `cache`."""
    if ni_uuid in cache:
        return cache[ni_uuid], None
    data, err = zedcloud_get(args, f"/api/v1/netinsts/id/{ni_uuid}")
    if err:
        cache[ni_uuid] = None
        return None, err
    ni = NiInfo(
        uuid=data.get("id", ni_uuid),
        name=data.get("name", ""),
        kind=data.get("kind", ""),
        port=data.get("port"),
    )
    cache[ni_uuid] = ni
    return ni, None


def guess_ni_kind_from_bridge(bridge_name: Optional[str]) -> Optional[str]:
    """Heuristic for when Zedcloud cannot be reached: 'bnN' = LOCAL, 'ethN' = SWITCH."""
    if not bridge_name:
        return None
    if re.match(r"^bn\d+$", bridge_name):
        return "NETWORK_INSTANCE_KIND_LOCAL"
    if re.match(r"^eth\d+$", bridge_name):
        return "NETWORK_INSTANCE_KIND_SWITCH"
    return None


def find_app_uuid(node_status: dict, name_substr: Optional[str] = None) -> Optional[str]:
    """Look up the first app instance the node reports."""
    raw = node_status.get("rawStatus")
    if not raw:
        return None
    try:
        raw_d = json.loads(raw) if isinstance(raw, str) else raw
    except Exception:
        return None
    apps = raw_d.get("appInstances") or []
    if not apps:
        return None
    if name_substr:
        for a in apps:
            if name_substr in (a.get("name") or ""):
                return a.get("uuid")
    return apps[0].get("uuid")


# ---------------------------------------------------------------------------
# Comparison primitives
# ---------------------------------------------------------------------------

def macs_equal(a: Optional[str], b: Optional[str]) -> bool:
    if not a or not b:
        return False
    return a.lower().strip() == b.lower().strip()


def ip4_only(addrs: list[str]) -> list[str]:
    return [a.split("/", 1)[0] for a in addrs if ":" not in a]
