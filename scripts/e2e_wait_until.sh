#!/usr/bin/env bash
#
# scripts/e2e_wait_until.sh — end-to-end lifecycle test for the
# zedamigo_wait_until resource.
#
# Fully self-contained and rootless. It:
#   * builds the provider from source into a temp dir;
#   * points OpenTofu/Terraform at it via a dev_overrides CLI config (so no
#     `tofu init` and no registry access are needed);
#   * stubs the executables the provider's Configure only LookPath-checks
#     (docker, qemu-system-x86_64, qemu-img, ip) — wait_until only ever runs
#     bash, so the stubs are never actually invoked.
#
# Like the host_reservation e2e and unlike the LAG one this needs no `unshare`:
# the resource only runs a script and writes files under `lib_path`.
#
# Every probe used here appends one byte to a counter file before deciding, so
# the number of times the script REALLY ran is observable from outside Terraform
# — which is how "a tuning-only change must not re-run the barrier" is asserted.
#
# Assertions: a probe that succeeds immediately creates the resource and records
# attempts/elapsed/stdout; the re-plan is clean (no perpetual diff); a probe that
# only succeeds later is actually retried; changing `interval` updates in place
# WITHOUT re-running the script; changing `script` forces replacement; a probe
# that never succeeds fails the apply with a timeout diagnostic and leaves the
# script + summary.txt + a bounded attempts/ tree behind on the target; a hung
# probe is bounded by `attempt_timeout`; and destroy removes the resource dir.
#
# Usage:  scripts/e2e_wait_until.sh
# Env:    TOFU=tofu|terraform   CLI to drive (default: tofu, else terraform)
#         KEEP=1                keep the temp dir on exit (for debugging)
#
# Requires: go, bash, and tofu or terraform. No root/sudo needed.
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

TMP="$(mktemp -d "${TMPDIR:-/tmp}/za-e2e-waituntil.XXXXXX")" || die "mktemp failed";
[ "${KEEP:-0}" = "1" ] || trap 'rm -rf "$TMP"' EXIT;

WORK="$TMP/work";
LIB="$TMP/lib";
STUBS="$TMP/stubs";
BIN="$TMP/bin";
FLAG="$TMP/flag";
COUNT="$TMP/count";
mkdir -p "$WORK" "$LIB" "$STUBS" "$BIN";
: > "$COUNT";

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
# haveflat is have() with all whitespace runs collapsed to single spaces, for
# assertions on diagnostic prose: Terraform hard-wraps diagnostic text to the
# terminal width, so a sentence can be split at an arbitrary word boundary.
haveflat(){ if grep -qF -- "$2" <<<"$(tr -s '[:space:]' ' ' <<<"$3")"; then ok "$1"; else bad "$1" "expected (whitespace-collapsed) substring: $2" "$3"; fi; }
eq(){ if [ "$2" = "$3" ]; then ok "$1"; else bad "$1" "expected: $3" "got: $2"; fi; }
ge(){ if [ "$2" -ge "$3" ]; then ok "$1"; else bad "$1" "expected >= $3" "got: $2"; fi; }
le(){ if [ "$2" -le "$3" ]; then ok "$1"; else bad "$1" "expected <= $3" "got: $2"; fi; }
tf(){ ( cd "$WORK" && "$TOFU" "$@"; ) 2>&1; }
tfout(){ ( cd "$WORK" && "$TOFU" output -raw "$1"; ) 2>/dev/null; }

# runs is how many times a probe script has actually been executed, counted on
# the target side rather than trusted from Terraform state.
runs(){ wc -c < "$COUNT" | tr -d ' '; }
res_dirs(){ find "$LIB/wait_until" -mindepth 1 -maxdepth 1 -type d 2>/dev/null | wc -l | tr -d ' '; }
only_res_dir(){ find "$LIB/wait_until" -mindepth 1 -maxdepth 1 -type d 2>/dev/null | head -1; }
attempt_dirs(){ find "$1/attempts" -mindepth 1 -maxdepth 1 -type d 2>/dev/null | wc -l | tr -d ' '; }

# cfg <resource-body>
cfg(){ cat > "$WORK/main.tf" <<EOF
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

output "attempts" {
  value = zedamigo_wait_until.probe.attempts
}

output "elapsed" {
  value = zedamigo_wait_until.probe.elapsed
}

output "stdout" {
  value = zedamigo_wait_until.probe.stdout
}

output "script_path" {
  value = zedamigo_wait_until.probe.script_path
}
EOF
}

# flag_probe <timeout> <interval> [variant]
# Succeeds only once "$FLAG" exists. `variant` changes the script BODY, which is
# how the forces-replacement case is exercised. Apart from the interpolated
# paths the script deliberately contains no '$', so it survives both the bash
# heredoc above and HCL interpolation untouched.
flag_probe(){ cat <<EOF
resource "zedamigo_wait_until" "probe" {
  timeout  = "$1"
  interval = "$2"
  script = <<-EOT
    # probe variant: ${3:-base}
    printf 'x' >> $COUNT
    if test -f $FLAG; then
      echo "flag is present"
      exit 0
    fi
    echo "flag is missing" >&2
    exit 1
  EOT
}
EOF
}

note "STEP 1: apply with a probe that succeeds on the first attempt";
: > "$FLAG";
cfg "$(flag_probe 2m 2s)";
out="$(tf apply -auto-approve -no-color)";
have "apply completes" "Apply complete" "$out";
eq "attempts recorded as 1" "$(tfout attempts)" "1";
eq "script ran exactly once" "$(runs)" "1";
have "stdout captured" "flag is present" "$(tfout stdout)";
SCRIPT_PATH="$(tfout script_path)";
if [ -x "$SCRIPT_PATH" ]; then ok "script written executable on the target"; else bad "script written executable on the target" "not executable: $SCRIPT_PATH"; fi
RES_DIR="$(dirname "$SCRIPT_PATH")";
if [ -s "$RES_DIR/summary.txt" ]; then ok "summary.txt written"; else bad "summary.txt written" "missing or empty: $RES_DIR/summary.txt"; fi
have "summary records success" "outcome:         succeeded" "$(cat "$RES_DIR/summary.txt")";
if [ -f "$RES_DIR/config_source_dir.tf" ]; then ok "TF back-pointer written"; else bad "TF back-pointer written" "missing"; fi
eq "one attempt log dir" "$(attempt_dirs "$RES_DIR")" "1";

note "STEP 2: re-plan is clean (no perpetual diff)";
have "no changes after apply" "No changes" "$(tf plan -no-color)";
eq "refresh did not re-run the script" "$(runs)" "1";

note "STEP 3: changing only 'interval' updates in place and does NOT re-run";
cfg "$(flag_probe 2m 3s)";
plan="$(tf plan -no-color)";
have "plan is an in-place update" "will be updated in-place" "$plan";
hasnt "plan is not a replacement" "must be replaced" "$plan";
have "in-place apply completes" "Apply complete" "$(tf apply -auto-approve -no-color)";
eq "script still ran only once" "$(runs)" "1";
eq "recorded attempts preserved" "$(tfout attempts)" "1";
eq "resource id/dir unchanged" "$(tfout script_path)" "$SCRIPT_PATH";

note "STEP 4: changing 'script' forces replacement and re-runs the barrier";
cfg "$(flag_probe 2m 3s variant-2)";
plan="$(tf plan -no-color)";
have "plan forces replacement" "must be replaced" "$plan";
have "replace apply completes" "Apply complete" "$(tf apply -auto-approve -no-color)";
eq "script ran a second time" "$(runs)" "2";
eq "old resource dir was destroyed" "$(res_dirs)" "1";

note "STEP 5: the probe is actually RETRIED until it succeeds";
tf destroy -auto-approve -no-color >/dev/null;
rm -f "$FLAG";
: > "$COUNT";
( sleep 6; : > "$FLAG"; ) &
FLAGGER=$!;
cfg "$(flag_probe 2m 1s)";
out="$(tf apply -auto-approve -no-color)";
wait "$FLAGGER" 2>/dev/null;
have "delayed-flag apply completes" "Apply complete" "$out";
ge "more than one attempt was made" "$(tfout attempts)" "2";
eq "attempts matches the real run count" "$(tfout attempts)" "$(runs)";
RES_DIR="$(dirname "$(tfout script_path)")";
# The first attempt plus at most the retained window plus the successful one.
le "attempt logs are pruned" "$(attempt_dirs "$RES_DIR")" "7";
if [ -d "$RES_DIR/attempts/0001" ]; then ok "first attempt's logs kept"; else bad "first attempt's logs kept" "attempts/0001 pruned"; fi

note "STEP 6: destroy removes the resource dir";
have "destroy completes" "Destroy complete" "$(tf destroy -auto-approve -no-color)";
eq "no resource dirs left" "$(res_dirs)" "0";

note "STEP 7: a probe that never succeeds FAILS the apply and leaves evidence";
rm -f "$FLAG";
: > "$COUNT";
cfg "$(flag_probe 6s 1s)";
out="$(tf apply -auto-approve -no-color)";
hasnt "apply did not complete" "Apply complete" "$out";
haveflat "timeout diagnostic" "Timed out waiting for the condition" "$out";
haveflat "diagnostic reports the exit code" "last attempt exit code: 1" "$out";
haveflat "diagnostic carries the probe's stderr" "flag is missing" "$out";
haveflat "diagnostic points at the target dir" "kept on the target under" "$out";
ge "the probe was retried before giving up" "$(runs)" "3";
RES_DIR="$(only_res_dir)";
if [ -n "$RES_DIR" ]; then ok "resource dir kept as post-mortem"; else bad "resource dir kept as post-mortem" "no dir under $LIB/wait_until"; fi
have "summary records the timeout" "outcome:         timed out" "$(cat "$RES_DIR/summary.txt")";
if [ -f "$RES_DIR/wait_until.sh" ]; then ok "script kept as post-mortem"; else bad "script kept as post-mortem" "missing"; fi
le "attempt logs stayed bounded" "$(attempt_dirs "$RES_DIR")" "6";
have "no state was written for the failed barrier" "No changes" "$(tf plan -destroy -no-color)";
rm -rf "$LIB/wait_until";

note "STEP 8: attempt_timeout bounds a hung probe";
: > "$COUNT";
cfg "$(cat <<EOF
resource "zedamigo_wait_until" "probe" {
  timeout         = "6s"
  interval        = "1s"
  attempt_timeout = "1s"
  script = <<-EOT
    printf 'x' >> $COUNT
    sleep 30
  EOT
}
EOF
)";
start="$(date +%s)";
out="$(tf apply -auto-approve -no-color)";
elapsed=$(( $(date +%s) - start ));
hasnt "hung-probe apply did not complete" "Apply complete" "$out";
haveflat "hung-probe apply times out" "Timed out waiting for the condition" "$out";
haveflat "diagnostic reports the abandoned attempt" "last attempt: abandoned" "$out";
haveflat "diagnostic warns the process may linger" "may still be running on the target" "$out";
ge "several attempts were started" "$(runs)" "2";
# 6s timeout: must not have waited for the 30s sleep to finish.
le "the overall timeout was honored" "$elapsed" "20";
rm -rf "$LIB/wait_until";

note "RESULT: $PASS passed, $FAIL failed";
if [ "$FAIL" -eq 0 ]; then echo "==> E2E PASSED"; exit 0; else echo "==> E2E FAILED"; exit 1; fi
