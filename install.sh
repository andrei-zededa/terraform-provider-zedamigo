#!/usr/bin/env sh

set -eu;

# path_prepend prepends $1 to PATH if it's a directory and not already in PATH.
# It persists the new PATH value in `.bashrc`.
path_prepend() {
	# shellcheck disable=SC3043
	local p="${1:-}";
	# shellcheck disable=SC3043
	local system="${2:-}";

	[ -n "$p" ] || return 0;
	[ -d "$p" ] || return 0;

	case ":$PATH:" in
		*":$p:"*)
			# already present — do nothing.
			;;
		*)
			PATH="$p${PATH:+:$PATH}";
			export PATH;
			;;
	esac

	rc="";
	[ -f "$HOME/.profile" ] && rc="$HOME/.profile";
	[ -f "$HOME/.bash_profile" ] && rc="$HOME/.bash_profile";
	[ -n "$rc" ] && {
		printf '\n# Added by zedamigo install script on %s.\n' "$(date)";
		# shellcheck disable=SC2016
		printf 'case ":$PATH:" in\n';
		printf '	*":%s:"*)\n' "$p";
		printf '		# already present — do nothing.\n';
		printf '		;;\n';
		printf '	*)\n';
		# shellcheck disable=SC2016
		printf '		PATH="%s${PATH:+:$PATH}";\n' "$p";
		printf '		export PATH;\n';
		printf '		;;\n';
		printf 'esac\n';
		printf '# End of section added by zedamigo install script.\n';
	} >> "$rc";
}

_uname="$(command -v uname)";

SYSTEM="linux";
ARCH="amd64";
INSTALL_VERSION="${1:-latest}";

kernel_name="$($_uname -s)";
case "$kernel_name" in
  Linux)
	SYSTEM="linux";
	ARCH="amd64";
	;;
  Darwin)
	SYSTEM="darwin";
	ARCH="arm64";
	;;
  *)
	>&2 printf "Unkown kernel name '%s', defaulting to system to linux.\n" "$kernel_name";
	;;
esac

_curl="$(command -v curl) -fsSL";

>&2 printf "Trying to install version '%s' of the zedamigo terraform provider from https://github.com/andrei-zededa/terraform-provider-zedamigo ($SYSTEM / $ARCH).\n" "$INSTALL_VERSION";

INSTALL_PATH="$HOME/.terraform.d/plugins/localhost/andrei-zededa/zedamigo/";
mkdir -p "$INSTALL_PATH";

version="$($_curl -o - "https://api.github.com/repos/andrei-zededa/terraform-provider-zedamigo/releases/$INSTALL_VERSION"	\
	| grep -E '^[[:space:]]+"name":[[:space:]]+"v[[:digit:]]+\.[[:digit:]]+\.[[:digit:]]+",$'				\
	| head -n1														\
	| sed -E 's/^[[:space:]]+"name":[[:space:]]+"v([[:digit:]]+\.[[:digit:]]+\.[[:digit:]]+)",$/\1/g')";

$_curl -o "$INSTALL_PATH/terraform-provider-zedamigo_${version}_${SYSTEM}_${ARCH}.zip"	\
	"https://github.com/andrei-zededa/terraform-provider-zedamigo/releases/download/v${version}/terraform-provider-zedamigo_${version}_${SYSTEM}_${ARCH}.zip";

_mktemp="$(command -v mktemp)";
TEMP_DIR="$($_mktemp -d)";

_tf="$(command -v opentofu || command -v tofu || command -v terraform || echo "")";

[ -z "$_tf" ] && {
	>&2 printf "No terraform or opentofu found. Will try to install the latest opentofu release from https://github.com/opentofu/opentofu .\n";

	tf_version="$($_curl -o - "https://api.github.com/repos/opentofu/opentofu/releases/latest"			\
		| grep -E '^[[:space:]]+"name":[[:space:]]+"v[[:digit:]]+\.[[:digit:]]+\.[[:digit:]]+",$'		\
		| head -n1												\
		| sed -E 's/^[[:space:]]+"name":[[:space:]]+"v([[:digit:]]+\.[[:digit:]]+\.[[:digit:]]+)",$/\1/g')";

	INSTALL_PATH="$HOME/bin";
	mkdir -p "$INSTALL_PATH";

	tofu_file="tofu_${tf_version}_${SYSTEM}_${ARCH}.tar.gz";

	$_curl -o "$TEMP_DIR/$tofu_file"	\
		"https://github.com/opentofu/opentofu/releases/download/v${tf_version}/$tofu_file";

	tar -xzf "$TEMP_DIR/$tofu_file" -C "$TEMP_DIR";
	mv "$TEMP_DIR/tofu" "$INSTALL_PATH/";
	ln -s "$INSTALL_PATH/tofu" "$INSTALL_PATH/tf";

	path_prepend "$INSTALL_PATH";

	_tf="$INSTALL_PATH/tf";
}

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

INITIAL_DIR="$PWD";
cd "$TEMP_DIR";
$_tf init;

cd "$INITIAL_DIR";
rm -rf "$TEMP_DIR";
