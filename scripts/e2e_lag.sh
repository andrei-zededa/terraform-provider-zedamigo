#!/usr/bin/env bash
#
# scripts/e2e_lag.sh — end-to-end lifecycle test for the zedamigo_lag resource.
#
# Fully self-contained and rootless. It:
#   * builds the provider from source into a temp dir;
#   * points OpenTofu/Terraform at it via a dev_overrides CLI config (so no
#     `tofu init` and no registry access are needed);
#   * stubs the executables the provider's Configure only LookPath-checks
#     (docker, qemu-system-x86_64, qemu-img) — a LAG only ever runs `ip`, so
#     the stubs are never actually invoked;
#   * exercises the whole lifecycle inside `unshare -rn` (a rootless user+net
#     namespace, which grants CAP_NET_ADMIN and lets the kernel autoload the
#     `bonding` module) against throwaway dummy NICs.
#
# Assertions: create + clean re-plan (no perpetual diff), in-place add/remove of
# members, external-drift self-heal, mode-change forces replacement, and
# idempotent destroy that leaves the member interfaces intact.
#
# Usage:  scripts/e2e_lag.sh
# Env:    TOFU=tofu|terraform   CLI to drive (default: tofu, else terraform)
#         KEEP=1                keep the temp dir on exit (for debugging)
#
# Requires: go, unshare (with unprivileged user namespaces enabled), ip,
#           and tofu or terraform. No root/sudo needed.
#
# Style: every statement ends with ';' unless it already closes with a block
# token ('}', 'fi', 'done'); every parameter expansion is double-quoted.

set -u;

PROVIDER_ADDR="registry.opentofu.org/andrei-zededa/zedamigo";
BOND="test-bond-0";

# ============================================================================
# Inner phase — runs INSIDE `unshare -rn`. Expects the ZA_* env vars that the
# outer phase exports below.
# ============================================================================
if [ "${ZA_E2E_INNER:-}" = "1" ]; then
    export TF_CLI_CONFIG_FILE="$ZA_TFRC";
    export TF_IN_AUTOMATION=1;
    export PATH="$ZA_STUBS:$ZA_IPDIR:/usr/sbin:/usr/bin:/bin";
    IP="$ZA_IP";
    WORK="$ZA_WORK";
    LIB="$ZA_LIB";
    TOFU="$ZA_TOFU";
    PASS=0;
    FAIL=0;

    note(){ printf '\n========== %s ==========\n' "$*"; }
    ok(){ echo "PASS: $1"; PASS=$((PASS + 1)); }
    bad(){ echo "FAIL: $1"; FAIL=$((FAIL + 1)); shift; printf '   %s\n' "$@"; }
    have(){ if grep -qF -- "$2" <<<"$3"; then ok "$1"; else bad "$1" "expected substring: $2" "$3"; fi; }
    hasnt(){ if grep -qF -- "$2" <<<"$3"; then bad "$1" "unexpected substring: $2" "$3"; else ok "$1"; fi; }
    member(){ "$IP" link show master "$BOND" 2>/dev/null | grep -qw "$1"; }
    tf(){ ( cd "$WORK" && "$TOFU" "$@"; ) 2>&1; }

    prov(){ cat > "$WORK/main.tf" <<EOF
terraform {
  required_providers {
    zedamigo = {
      source = "andrei-zededa/zedamigo"
    }
  }
}

provider "zedamigo" {
  use_sudo = false
  lib_path = "$LIB"
}

$1
EOF
    }
    cfg_lacp(){ prov "resource \"zedamigo_lag\" \"b\" {
  name                = \"$BOND\"
  mode                = \"802.3ad\"
  miimon              = 100
  lacp_rate           = \"fast\"
  xmit_hash_policy    = \"layer3+4\"
  mtu                 = 1500
  state               = \"up\"
  enslaved_interfaces = $1
}"; }
    cfg_rr(){ prov "resource \"zedamigo_lag\" \"b\" {
  name                = \"$BOND\"
  mode                = \"balance-rr\"
  miimon              = 100
  mtu                 = 1500
  state               = \"up\"
  enslaved_interfaces = $1
}"; }

    "$IP" link set lo up 2>/dev/null;
    "$IP" link add m0 type dummy;
    "$IP" link add m1 type dummy;
    echo "dummy members:";
    "$IP" -br link show type dummy;

    note "STEP 1: create bond (802.3ad) with member m0";
    cfg_lacp '["m0"]';
    have "apply1 completes" "Apply complete" "$(tf apply -auto-approve -no-color)";
    have "bond mode 802.3ad" "bond mode 802.3ad" "$("$IP" -d link show "$BOND" 2>&1)";
    have "lacp_rate fast applied" "lacp_rate fast" "$("$IP" -d link show "$BOND" 2>&1)";
    if member m0; then ok "m0 enslaved"; else bad "m0 enslaved" "$("$IP" link show master "$BOND" 2>&1)"; fi
    if member m1; then bad "m1 not enslaved" "m1 unexpectedly a member"; else ok "m1 not enslaved"; fi

    note "STEP 1b: re-plan is clean (no perpetual diff)";
    have "no changes after apply" "No changes" "$(tf plan -no-color)";

    note "STEP 2: add m1 in-place (members = m0, m1)";
    cfg_lacp '["m0", "m1"]';
    out="$(tf plan -no-color)";
    have "plan2 is update-in-place" "will be updated in-place" "$out";
    hasnt "plan2 is NOT a replace" "must be replaced" "$out";
    have "apply2 completes" "Apply complete" "$(tf apply -auto-approve -no-color)";
    if member m0; then ok "m0 still a member"; else bad "m0 still a member" "$("$IP" link show master "$BOND" 2>&1)"; fi
    if member m1; then ok "m1 now a member"; else bad "m1 now a member" "$("$IP" link show master "$BOND" 2>&1)"; fi

    note "STEP 3: remove m0 in-place (members = m1)";
    cfg_lacp '["m1"]';
    have "plan3 is update-in-place" "will be updated in-place" "$(tf plan -no-color)";
    have "apply3 completes" "Apply complete" "$(tf apply -auto-approve -no-color)";
    if member m1; then ok "m1 still a member"; else bad "m1 still a member" "$("$IP" link show master "$BOND" 2>&1)"; fi
    if member m0; then bad "m0 released" "m0 still a member"; else ok "m0 released"; fi
    if "$IP" link show m0 >/dev/null 2>&1; then ok "released m0 still exists"; else bad "released m0 still exists" "m0 is gone"; fi

    note "STEP 4: external drift (nomaster m1) self-heals in-place";
    "$IP" link set m1 nomaster;
    if member m1; then bad "drift created" "m1 still a member"; else ok "drift created (m1 removed externally)"; fi
    have "drift plan is update-in-place" "will be updated in-place" "$(tf plan -no-color)";
    tf apply -auto-approve -no-color >/dev/null;
    if member m1; then ok "m1 re-enslaved after drift"; else bad "m1 re-enslaved after drift" "$("$IP" link show master "$BOND" 2>&1)"; fi

    note "STEP 5: mode change forces replacement";
    cfg_rr '["m1"]';
    out="$(tf plan -no-color)";
    have "mode change forces replace" "must be replaced" "$out";
    have "forces replacement marker" "forces replacement" "$out";
    have "apply5 completes" "Apply complete" "$(tf apply -auto-approve -no-color)";
    have "bond now balance-rr" "bond mode balance-rr" "$("$IP" -d link show "$BOND" 2>&1)";
    if member m1; then ok "m1 member after replace"; else bad "m1 member after replace" "$("$IP" link show master "$BOND" 2>&1)"; fi

    note "STEP 6: destroy";
    have "destroy completes" "Destroy complete" "$(tf destroy -auto-approve -no-color)";
    if "$IP" link show "$BOND" >/dev/null 2>&1; then bad "bond removed" "bond still present"; else ok "bond removed"; fi
    if "$IP" link show m1 >/dev/null 2>&1; then ok "member m1 survives destroy"; else bad "member m1 survives destroy" "m1 is gone"; fi

    note "RESULT: $PASS passed, $FAIL failed";
    if [ "$FAIL" -eq 0 ]; then exit 0; else exit 1; fi
fi

# ============================================================================
# Outer phase — prerequisite checks, build, dev_overrides, stubs, then re-exec
# this same script inside `unshare -rn`.
# ============================================================================
die(){ echo "ERROR: $*" >&2; exit 1; }

REPO_ROOT="$(git -C "$(dirname "${BASH_SOURCE[0]}")" rev-parse --show-toplevel 2>/dev/null)" || REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)";
[ -f "$REPO_ROOT/main.go" ] || die "can't locate repo root (main.go not found near $REPO_ROOT)";

TOFU="${TOFU:-}";
if [ -z "$TOFU" ]; then
    if command -v tofu >/dev/null 2>&1; then
        TOFU="tofu";
    elif command -v terraform >/dev/null 2>&1; then
        TOFU="terraform";
    else
        die "neither 'tofu' nor 'terraform' found on PATH (set TOFU=...)";
    fi
fi
command -v "$TOFU" >/dev/null 2>&1 || die "'$TOFU' not found on PATH";
command -v go >/dev/null 2>&1 || die "'go' not found on PATH";
command -v unshare >/dev/null 2>&1 || die "'unshare' not found on PATH";
IP_BIN="$(command -v ip)" || die "'ip' not found on PATH";

unshare -rn true 2>/dev/null || die "rootless 'unshare -rn' is not permitted (unprivileged user namespaces disabled?)";

TMP="$(mktemp -d "${TMPDIR:-/tmp}/za-e2e-lag.XXXXXX")" || die "mktemp failed";
[ "${KEEP:-0}" = "1" ] || trap 'rm -rf "$TMP"' EXIT;

echo "==> repo:  $REPO_ROOT";
echo "==> tofu:  $TOFU ($("$TOFU" version 2>/dev/null | head -1))";
echo "==> tmp:   $TMP";
echo "==> building provider ...";
mkdir -p "$TMP/bin" "$TMP/stubs" "$TMP/work" "$TMP/lib";
( cd "$REPO_ROOT" && go build -o "$TMP/bin/terraform-provider-zedamigo" .; ) || die "go build failed";

cat > "$TMP/dev.tfrc" <<EOF
provider_installation {
  dev_overrides {
    "$PROVIDER_ADDR" = "$TMP/bin"
  }
  direct {}
}
EOF

for t in docker qemu-system-x86_64 qemu-img; do
    printf '#!/bin/sh\nexit 0\n' > "$TMP/stubs/$t";
    chmod +x "$TMP/stubs/$t";
done

export ZA_E2E_INNER=1;
export ZA_TFRC="$TMP/dev.tfrc";
export ZA_STUBS="$TMP/stubs";
export ZA_WORK="$TMP/work";
export ZA_LIB="$TMP/lib";
export ZA_TOFU="$TOFU";
export ZA_IP="$IP_BIN";
ZA_IPDIR="$(dirname "$IP_BIN")";
export ZA_IPDIR;

echo "==> running lifecycle inside 'unshare -rn' ...";
unshare -rn bash "${BASH_SOURCE[0]}";
rc="$?";

echo "";
if [ "$rc" -eq 0 ]; then echo "==> E2E PASSED"; else echo "==> E2E FAILED (rc=$rc)"; fi
exit "$rc";
