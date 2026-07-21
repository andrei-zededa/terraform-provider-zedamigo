#!/usr/bin/env bash
# host_reservation.bash — atomic host-capacity reservation backing the
# zedamigo_host_reservation Terraform resource.
#
# Capacity is declared by the operator pre-creating EMPTY slot files under the
# reservations root (<root>); this script only claims among existing files and
# never invents capacity. A slot file that is empty (`! -s`) is FREE; one whose
# content equals a reservation id is RESERVED by that reservation.
#
#   <root>/cpus/unit/<coreID>   one file per reservable CPU (filename = core ID)
#   <root>/ram/gb/<index>       one file per reservable GB
#   <root>/devs/<abs-dev-path>  e.g. /dev/sdb -> <root>/devs/dev/sdb
#   <root>/.lock                provider-owned lock file (NOT capacity)
#
# It is always invoked as ONE argv so the entire find-free-and-claim-or-rollback
# critical section runs under a single flock in one Executor.Run call (an flock
# FD cannot survive across the short-lived provider process / separate calls):
#
#   bash -c "$SCRIPT" za-host-reservation <mode> <flockbin> <id> <root> [extra...]
#
# Modes:
#   reserve <flockbin> <id> <root> <ncpu> <nmem> [dev...]
#   release <flockbin> <id> <root>
#   scan    <flockbin> <id> <root>
#
# All dynamic values arrive as positional parameters ("$@") and are NEVER
# interpolated into code — only referenced as quoted parameter expansions. This
# is injection-safe on the Local executor (argv, no shell) and the SSH executor
# (each arg single-quoted by shellQuoteArg, then re-parsed by the login shell).
#
# Output: JSON on stdout for success — {"cpus":[..],"mem":[..],"devs":[".."]} ;
# a human-readable message on stderr for failure. Exit codes:
#   0 ok   1 usage   2 root missing   3 no capacity dir   4 insufficient CPUs
#   5 insufficient RAM   6 lock error   7 device has no capacity file
#   8 device already reserved   9 commit/rollback failure

set -u
export PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:${PATH:-}"

LOCK_WAIT=15

mode="${1:-}"
flockbin="${2:-}"
id="${3:-}"
root="${4:-}"
if [ -z "$mode" ] || [ -z "$id" ] || [ -z "$root" ]; then
	printf 'internal error: missing mode/id/root\n' >&2
	exit 1
fi
root="${root%/}"
shift 4

fail() {
	local code="$1"
	shift
	printf '%s\n' "$*" >&2
	exit "$code"
}

# acquire_lock <required 0|1>: open fd 9 on <root>/.lock and take an exclusive,
# time-bounded flock. When flock is unavailable or the lock can't be taken, a
# required lock fails (exit 6) while a best-effort lock (release) proceeds.
acquire_lock() {
	local required="$1"
	if [ -z "$flockbin" ]; then
		[ "$required" = 1 ] && fail 6 "flock is not available on the target (install util-linux)"
		return 0
	fi
	# `<>` opens read-write and creates the lock file if absent; we never
	# truncate or unlink it, so its inode (and thus the lock domain) is stable.
	if ! exec 9<>"$root/.lock"; then
		[ "$required" = 1 ] && fail 6 "cannot open lock file $root/.lock (is $root writable?)"
		return 0
	fi
	if ! "$flockbin" -w "$LOCK_WAIT" 9; then
		[ "$required" = 1 ] && fail 6 "could not acquire reservation lock within ${LOCK_WAIT}s: $root/.lock"
		return 0
	fi
	return 0
}

# json_str <s>: emit s as a JSON string literal (escape backslash and quote).
json_str() {
	local s="$1"
	s="${s//\\/\\\\}"
	s="${s//\"/\\\"}"
	printf '"%s"' "$s"
}

# emit_owned: scan the whole tree for files whose content == $id and print the
# canonical JSON. Shared by reserve (post-commit) and scan so Create and Read
# produce byte-identical output (no spurious first-refresh drift).
emit_owned() {
	local f b p i
	local -a c=() m=() d=()
	if [ -d "$root/cpus/unit" ]; then
		for f in "$root/cpus/unit"/*; do
			[ -f "$f" ] || continue
			b="${f##*/}"
			case "$b" in '' | *[!0-9]*) continue ;; esac
			[ "$(cat "$f" 2>/dev/null)" = "$id" ] && c+=("$b")
		done
	fi
	if [ -d "$root/ram/gb" ]; then
		for f in "$root/ram/gb"/*; do
			[ -f "$f" ] || continue
			b="${f##*/}"
			case "$b" in '' | *[!0-9]*) continue ;; esac
			[ "$(cat "$f" 2>/dev/null)" = "$id" ] && m+=("$b")
		done
	fi
	if [ -d "$root/devs" ]; then
		while IFS= read -r p; do
			[ -n "$p" ] || continue
			[ "$(cat "$root/devs/$p" 2>/dev/null)" = "$id" ] && d+=("/$p")
		done < <(find "$root/devs" -type f -printf '%P\n' 2>/dev/null)
	fi
	[ "${#c[@]}" -gt 0 ] && mapfile -t c < <(printf '%s\n' "${c[@]}" | sort -n)
	[ "${#m[@]}" -gt 0 ] && mapfile -t m < <(printf '%s\n' "${m[@]}" | sort -n)
	[ "${#d[@]}" -gt 0 ] && mapfile -t d < <(printf '%s\n' "${d[@]}" | LC_ALL=C sort)

	local ci mi dj
	ci="$(
		IFS=,
		printf '%s' "${c[*]-}"
	)"
	mi="$(
		IFS=,
		printf '%s' "${m[*]-}"
	)"
	local -a dq=()
	for ((i = 0; i < ${#d[@]}; i++)); do
		dq+=("$(json_str "${d[i]}")")
	done
	dj="$(
		IFS=,
		printf '%s' "${dq[*]-}"
	)"
	printf '{"cpus":[%s],"mem":[%s],"devs":[%s]}\n' "$ci" "$mi" "$dj"
}

# select_free <dir> <want> <label> <ecode>: append the <want> lowest-numbered
# FREE (empty) slot files under <dir> to the global `claim` array. Read-only —
# it decides what to claim but writes nothing.
select_free() {
	local dir="$1" want="$2" label="$3" ecode="$4"
	[ "$want" -gt 0 ] || return 0
	[ -d "$dir" ] || fail 3 "no $label capacity declared (missing directory $dir); the operator must pre-create slot files"
	local -a ids=()
	local f b i s n=0
	for f in "$dir"/*; do
		[ -f "$f" ] || continue
		b="${f##*/}"
		case "$b" in '' | *[!0-9]*) continue ;; esac
		ids+=("$b")
	done
	[ "${#ids[@]}" -gt 0 ] && mapfile -t ids < <(printf '%s\n' "${ids[@]}" | sort -n)
	for ((i = 0; i < ${#ids[@]}; i++)); do
		s="${ids[i]}"
		[ -s "$dir/$s" ] && continue # non-empty => reserved, skip
		claim+=("$dir/$s")
		n=$((n + 1))
		[ "$n" -eq "$want" ] && break
	done
	[ "$n" -eq "$want" ] || fail "$ecode" "insufficient free $label under $dir: requested $want, only $n free"
}

do_reserve() {
	local ncpu="${1:-0}" nmem="${2:-0}"
	shift 2
	case "$ncpu" in '' | *[!0-9]*) fail 1 "internal error: cpu count not numeric: '$ncpu'" ;; esac
	case "$nmem" in '' | *[!0-9]*) fail 1 "internal error: mem count not numeric: '$nmem'" ;; esac
	[ -d "$root" ] || fail 2 "reservations path does not exist: $root"

	acquire_lock 1

	# --- selection phase (read-only; all categories fully validated first) ---
	claim=()
	select_free "$root/cpus/unit" "$ncpu" "CPUs" 4
	select_free "$root/ram/gb" "$nmem" "RAM" 5
	local dev slot owner
	for dev in "$@"; do
		slot="${root}/devs${dev}"
		[ -e "$slot" ] || fail 7 "requested device is not reservable (no capacity file): $dev (expected $slot); the operator must pre-create it"
		[ -f "$slot" ] || fail 7 "requested device capacity path is not a regular file: $slot"
		if [ -s "$slot" ]; then
			owner="$(cat "$slot" 2>/dev/null)"
			fail 8 "requested device already reserved by reservation '$owner': $dev"
		fi
		claim+=("$slot")
	done

	# --- commit phase (all-or-nothing) ---
	local -a done_files=()
	local target i j
	for ((i = 0; i < ${#claim[@]}; i++)); do
		target="${claim[i]}"
		if ! printf '%s' "$id" >"$target" 2>/dev/null; then
			for ((j = 0; j < ${#done_files[@]}; j++)); do
				: >"${done_files[j]}" 2>/dev/null || true
			done
			fail 9 "failed to write reservation marker to $target (rolled back all claims for this reservation)"
		fi
		done_files+=("$target")
	done

	emit_owned
}

do_release() {
	[ -d "$root" ] || exit 0 # capacity tree gone => nothing to release
	acquire_lock 0           # best-effort: clearing our own markers is race-safe
	local f
	while IFS= read -r f; do
		[ -n "$f" ] || continue
		if [ "$(cat "$f" 2>/dev/null)" = "$id" ]; then
			: >"$f" 2>/dev/null || true
		fi
	done < <(grep -rlF -- "$id" "$root/cpus" "$root/ram" "$root/devs" 2>/dev/null || true)
	exit 0
}

do_scan() {
	if [ ! -d "$root" ]; then
		printf '{"cpus":[],"mem":[],"devs":[]}\n'
		exit 0
	fi
	emit_owned
	exit 0
}

case "$mode" in
reserve) do_reserve "$@" ;;
release) do_release ;;
scan) do_scan ;;
*) fail 1 "unknown mode: $mode" ;;
esac
