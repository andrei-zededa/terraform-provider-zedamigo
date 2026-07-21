#!/usr/bin/env bash
#
# scripts/e2e_host_reservation.sh — end-to-end lifecycle test for the
# zedamigo_host_reservation resource.
#
# Fully self-contained and rootless. It:
#   * builds the provider from source into a temp dir;
#   * points OpenTofu/Terraform at it via a dev_overrides CLI config (so no
#     `tofu init` and no registry access are needed);
#   * stubs the executables the provider's Configure only LookPath-checks
#     (docker, qemu-system-x86_64, qemu-img, ip) — host_reservation only ever
#     runs bash+flock, so the stubs are never actually invoked;
#   * pre-creates an operator-style capacity tree (8 CPUs, 16 GB, one device)
#     and drives the whole lifecycle against it. Unlike the LAG e2e this needs
#     no `unshare`: the resource is pure file I/O under `path`.
#
# Assertions: create + clean re-plan (no perpetual diff), a second reservation
# that would oversubscribe CPUs fails, a duplicate device reservation fails,
# destroy frees the reserved slots, freed slots are reclaimable, and a full
# destroy leaves the operator's capacity files intact.
#
# Usage:  scripts/e2e_host_reservation.sh
# Env:    TOFU=tofu|terraform   CLI to drive (default: tofu, else terraform)
#         KEEP=1                keep the temp dir on exit (for debugging)
#
# Requires: go, flock (util-linux), and tofu or terraform. No root/sudo needed.
#
# Style: every statement ends with ';' unless it already closes with a block
# token ('}', 'fi', 'done'); every parameter expansion is double-quoted.

set -u;

PROVIDER_ADDR="registry.opentofu.org/andrei-zededa/zedamigo";

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
command -v flock >/dev/null 2>&1 || die "'flock' not found on PATH (install util-linux)";

TMP="$(mktemp -d "${TMPDIR:-/tmp}/za-e2e-hostres.XXXXXX")" || die "mktemp failed";
[ "${KEEP:-0}" = "1" ] || trap 'rm -rf "$TMP"' EXIT;

WORK="$TMP/work";
LIB="$TMP/lib";
RES="$TMP/res";
STUBS="$TMP/stubs";
BIN="$TMP/bin";
mkdir -p "$WORK" "$LIB" "$STUBS" "$BIN" "$RES/cpus/unit" "$RES/ram/gb" "$RES/devs/dev";

echo "==> repo:  $REPO_ROOT";
echo "==> tofu:  $TOFU ($("$TOFU" version 2>/dev/null | head -1))";
echo "==> tmp:   $TMP";
echo "==> building provider ...";
( cd "$REPO_ROOT" && go build -o "$BIN/terraform-provider-zedamigo" .; ) || die "go build failed";

cat > "$TMP/dev.tfrc" <<EOF
provider_installation {
  dev_overrides {
    "$PROVIDER_ADDR" = "$BIN"
  }
  direct {}
}
EOF

for t in docker qemu-system-x86_64 qemu-img ip; do
    printf '#!/bin/sh\nexit 0\n' > "$STUBS/$t";
    chmod +x "$STUBS/$t";
done

# Operator-declared capacity: 8 reservable CPUs, 16 GB, one device.
for i in 0 1 2 3 4 5 6 7; do : > "$RES/cpus/unit/$i"; done
for i in $(seq 0 15); do : > "$RES/ram/gb/$i"; done
: > "$RES/devs/dev/sdb";

export TF_CLI_CONFIG_FILE="$TMP/dev.tfrc";
export TF_IN_AUTOMATION=1;
export PATH="$STUBS:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin";

PASS=0;
FAIL=0;
note(){ printf '\n========== %s ==========\n' "$*"; }
ok(){ echo "PASS: $1"; PASS=$((PASS + 1)); }
bad(){ echo "FAIL: $1"; FAIL=$((FAIL + 1)); shift; printf '   %s\n' "$@"; }
have(){ if grep -qF -- "$2" <<<"$3"; then ok "$1"; else bad "$1" "expected substring: $2" "$3"; fi; }
hasnt(){ if grep -qF -- "$2" <<<"$3"; then bad "$1" "unexpected substring: $2" "$3"; else ok "$1"; fi; }
eq(){ if [ "$2" = "$3" ]; then ok "$1"; else bad "$1" "expected: $3" "got: $2"; fi; }
tf(){ ( cd "$WORK" && "$TOFU" "$@"; ) 2>&1; }
reserved_cpus(){ find "$RES/cpus/unit" -type f -size +0c | wc -l | tr -d ' '; }
total_cpus(){ find "$RES/cpus/unit" -type f | wc -l | tr -d ' '; }

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
# res <name> <cpus> <mem> <devs-hcl>
res(){ printf 'resource "zedamigo_host_reservation" "%s" {\n  path = "%s"\n  cpus = %s\n  mem  = %s\n  devs = %s\n}\n' "$1" "$RES" "$2" "$3" "$4"; }

note "STEP 1: apply A (4 cpu, 4 gb, /dev/sdb)";
prov "$(res a 4 4 '["/dev/sdb"]')";
have "apply A completes" "Apply complete" "$(tf apply -auto-approve -no-color)";
eq "A reserved 4 cpus on disk" "$(reserved_cpus)" "4";
if [ -s "$RES/devs/dev/sdb" ]; then ok "/dev/sdb marked reserved"; else bad "/dev/sdb marked reserved" "capacity file still empty"; fi

note "STEP 2: re-plan is clean (no perpetual diff)";
have "no changes after apply" "No changes" "$(tf plan -no-color)";

note "STEP 3: apply A+B (b: 4 cpu)";
prov "$(res a 4 4 '["/dev/sdb"]')
$(res b 4 0 '[]')";
have "apply B completes" "Apply complete" "$(tf apply -auto-approve -no-color)";
eq "A+B reserved 8 cpus" "$(reserved_cpus)" "8";

note "STEP 4: apply C (1 cpu) FAILS — oversubscription prevented";
prov "$(res a 4 4 '["/dev/sdb"]')
$(res b 4 0 '[]')
$(res c 1 0 '[]')";
out="$(tf apply -auto-approve -no-color)";
have "C apply fails with insufficient CPUs" "insufficient free CPUs" "$out";
hasnt "C apply did not complete" "Apply complete" "$out";
eq "still only 8 cpus reserved" "$(reserved_cpus)" "8";

note "STEP 5: apply E (duplicate /dev/sdb) FAILS — double-use prevented";
prov "$(res a 4 4 '["/dev/sdb"]')
$(res b 4 0 '[]')
$(res e 0 0 '["/dev/sdb"]')";
out="$(tf apply -auto-approve -no-color)";
have "E apply fails: already reserved" "already reserved" "$out";

note "STEP 6: remove B — destroy frees its slots";
prov "$(res a 4 4 '["/dev/sdb"]')";
have "remove-B apply completes" "Apply complete" "$(tf apply -auto-approve -no-color)";
eq "back to 4 cpus reserved" "$(reserved_cpus)" "4";

note "STEP 7: apply C (4 cpu) reclaims the freed slots";
prov "$(res a 4 4 '["/dev/sdb"]')
$(res c 4 0 '[]')";
have "reclaim apply completes" "Apply complete" "$(tf apply -auto-approve -no-color)";
eq "8 cpus reserved again" "$(reserved_cpus)" "8";

note "STEP 8: destroy all — slots freed, capacity files preserved";
have "destroy completes" "Destroy complete" "$(tf destroy -auto-approve -no-color)";
eq "0 cpus reserved after destroy" "$(reserved_cpus)" "0";
eq "capacity files preserved" "$(total_cpus)" "8";
if [ -e "$RES/devs/dev/sdb" ]; then ok "device capacity file preserved"; else bad "device capacity file preserved" "capacity file gone"; fi
if [ -s "$RES/devs/dev/sdb" ]; then bad "device released" "device capacity file still non-empty"; else ok "device released"; fi

note "RESULT: $PASS passed, $FAIL failed";
if [ "$FAIL" -eq 0 ]; then echo "==> E2E PASSED"; exit 0; else echo "==> E2E FAILED"; exit 1; fi
