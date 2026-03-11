#!/usr/bin/env bash

set -eu

# --- Color helpers ---
_color=""
[ -t 2 ] && _color=1

info()  { [ -n "$_color" ] && printf '\033[1;34m[INFO]\033[0m %s\n'  "$*" >&2 || printf '[INFO] %s\n'  "$*" >&2; }
warn()  { [ -n "$_color" ] && printf '\033[1;33m[WARN]\033[0m %s\n'  "$*" >&2 || printf '[WARN] %s\n'  "$*" >&2; }
error() { [ -n "$_color" ] && printf '\033[1;31m[ERROR]\033[0m %s\n' "$*" >&2 || printf '[ERROR] %s\n' "$*" >&2; }

# --- Cleanup trap ---
TEMP_DIR=""
cleanup() {
	[ -n "$TEMP_DIR" ] && rm -rf "$TEMP_DIR"
}
trap cleanup EXIT

# --- Check required install utilities ---
missing=""
for cmd in curl grep sed mktemp unzip tar; do
	command -v "$cmd" >/dev/null 2>&1 || missing="$missing $cmd"
done
if [ -n "$missing" ]; then
	error "Missing required utilities:$missing"
	error "Please install them before running this script."
	exit 1
fi

# --- path_prepend ---
# Prepends $1 to PATH if it's a directory and not already in PATH.
# Persists the new PATH value in .bashrc/.profile.
path_prepend() {
	local p="${1:-}"
	local system="${2:-}"

	[ -n "$p" ] || return 0
	[ -d "$p" ] || return 0

	case ":$PATH:" in
		*":$p:"*)
			# already present — do nothing.
			;;
		*)
			PATH="$p${PATH:+:$PATH}"
			export PATH
			;;
	esac

	rc=""
	[ -f "$HOME/.profile" ] && rc="$HOME/.profile"
	[ -f "$HOME/.bash_profile" ] && rc="$HOME/.bash_profile"
	[ -n "$rc" ] && {
		printf '\n# Added by zedamigo install script on %s.\n' "$(date)"
		printf 'case ":$PATH:" in\n'
		printf '	*":%s:"*)\n' "$p"
		printf '		# already present — do nothing.\n'
		printf '		;;\n'
		printf '	*)\n'
		printf '		PATH="%s${PATH:+:$PATH}";\n' "$p"
		printf '		export PATH;\n'
		printf '		;;\n'
		printf 'esac\n'
		printf '# End of section added by zedamigo install script.\n'
	} >> "$rc"
}

# --- Detect OS and architecture ---
INSTALL_VERSION="${1:-latest}"

kernel_name="$(uname -s)"
machine="$(uname -m)"

case "$kernel_name" in
	Linux)  SYSTEM="linux" ;;
	Darwin) SYSTEM="darwin" ;;
	*)
		error "Unknown kernel name '$kernel_name'. Only Linux and macOS are supported."
		exit 1
		;;
esac

case "$machine" in
	x86_64)        ARCH="amd64" ;;
	aarch64|arm64) ARCH="arm64" ;;
	*)
		error "Unknown architecture '$machine'. Only x86_64 and arm64 are supported."
		exit 1
		;;
esac

info "Installing version '$INSTALL_VERSION' of the zedamigo terraform provider ($SYSTEM/$ARCH)."

# --- Fetch release version ---
version="$(curl -fsSL "https://api.github.com/repos/andrei-zededa/terraform-provider-zedamigo/releases/$INSTALL_VERSION" \
	| grep -E '^[[:space:]]+"name":[[:space:]]+"v[[:digit:]]+\.[[:digit:]]+\.[[:digit:]]+",$' \
	| head -n1 \
	| sed -E 's/^[[:space:]]+"name":[[:space:]]+"v([[:digit:]]+\.[[:digit:]]+\.[[:digit:]]+)",$/\1/g')"

if [ -z "$version" ]; then
	error "Could not determine release version. Check that the release '$INSTALL_VERSION' exists."
	exit 1
fi

info "Resolved version: v$version"

# --- Download and extract provider ---
TEMP_DIR="$(mktemp -d)"
PROVIDER_DIR="$HOME/.terraform.d/plugins/localhost/andrei-zededa/zedamigo/${version}/${SYSTEM}_${ARCH}"
mkdir -p "$PROVIDER_DIR"

zip_file="terraform-provider-zedamigo_${version}_${SYSTEM}_${ARCH}.zip"
curl -fsSL -o "$TEMP_DIR/$zip_file" \
	"https://github.com/andrei-zededa/terraform-provider-zedamigo/releases/download/v${version}/$zip_file"

unzip -o "$TEMP_DIR/$zip_file" -d "$PROVIDER_DIR"
chmod +x "$PROVIDER_DIR"/terraform-provider-zedamigo*

info "Provider extracted to $PROVIDER_DIR"

# --- Install OpenTofu if no terraform/tofu found ---
_tf="$(command -v opentofu || command -v tofu || command -v terraform || echo "")"

if [ -z "$_tf" ]; then
	info "No terraform or opentofu found. Installing the latest OpenTofu release."

	tf_version="$(curl -fsSL "https://api.github.com/repos/opentofu/opentofu/releases/latest" \
		| grep -E '^[[:space:]]+"name":[[:space:]]+"v[[:digit:]]+\.[[:digit:]]+\.[[:digit:]]+",$' \
		| head -n1 \
		| sed -E 's/^[[:space:]]+"name":[[:space:]]+"v([[:digit:]]+\.[[:digit:]]+\.[[:digit:]]+)",$/\1/g')"

	TOFU_INSTALL_PATH="$HOME/bin"
	mkdir -p "$TOFU_INSTALL_PATH"

	tofu_file="tofu_${tf_version}_${SYSTEM}_${ARCH}.tar.gz"
	curl -fsSL -o "$TEMP_DIR/$tofu_file" \
		"https://github.com/opentofu/opentofu/releases/download/v${tf_version}/$tofu_file"

	tar -xzf "$TEMP_DIR/$tofu_file" -C "$TEMP_DIR"
	mv "$TEMP_DIR/tofu" "$TOFU_INSTALL_PATH/"
	ln -sf "$TOFU_INSTALL_PATH/tofu" "$TOFU_INSTALL_PATH/tf"

	path_prepend "$TOFU_INSTALL_PATH"

	_tf="$TOFU_INSTALL_PATH/tf"
	info "OpenTofu v$tf_version installed to $TOFU_INSTALL_PATH"
fi

# --- Verify provider with tofu/terraform init ---
cat << EOF > "$TEMP_DIR/provider.tf"
terraform {
  required_providers {
    zedamigo = {
      source = "localhost/andrei-zededa/zedamigo"
      version = "${version}"
    }
  }
}

provider "zedamigo" {
  target = "localhost"
}
EOF

INITIAL_DIR="$PWD"
cd "$TEMP_DIR"
$_tf init
cd "$INITIAL_DIR"

# --- Runtime dependency advisory ---
info ""
info "=== Runtime Dependency Check ==="

check_cmd() {
	local cmd="$1"
	local label="${2:-$1}"
	local required="${3:-required}"
	if command -v "$cmd" >/dev/null 2>&1; then
		info "  $label: found ($(command -v "$cmd"))"
		return 0
	else
		if [ "$required" = "required" ]; then
			warn "  $label: NOT FOUND (required)"
		else
			warn "  $label: not found (optional)"
		fi
		return 1
	fi
}

if [ "$SYSTEM" = "linux" ]; then
	info "Checking Linux runtime dependencies..."
	check_cmd docker docker required || true
	check_cmd qemu-system-x86_64 qemu-system-x86_64 required || true
	check_cmd qemu-img qemu-img required || true
	check_cmd ip "ip (iproute2)" required || true
	check_cmd swtpm swtpm optional || true
	check_cmd genisoimage genisoimage optional || true
	check_cmd taskset taskset optional || true
elif [ "$SYSTEM" = "darwin" ]; then
	info "Checking macOS runtime dependencies..."
	check_cmd docker docker required || true
	check_cmd vfkit vfkit required || true
	check_cmd qemu-img qemu-img required || true
	check_cmd swtpm swtpm optional || true
	if command -v genisoimage >/dev/null 2>&1 || command -v mkisofs >/dev/null 2>&1; then
		info "  genisoimage/mkisofs: found"
	else
		warn "  genisoimage/mkisofs: not found (optional — brew install cdrtools provides mkisofs)"
	fi
	info ""
	warn "macOS note: Networking resources (bridge, tap, vlan, dhcp_server, dhcp6_server, radv) are not supported on macOS."
	warn "macOS note: Nested virtualization requires Apple M3 or later."
fi

info ""
info "Installation complete! Provider zedamigo v$version is ready to use."
