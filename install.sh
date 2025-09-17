#!/usr/bin/env sh

set -eu;

_curl="$(command -v curl) -fsSL";

INSTALL_PATH="$HOME/.terraform.d/plugins/localhost/andrei-zededa/zedamigo/";

mkdir -p "$INSTALL_PATH";

version="$($_curl -o - "https://api.github.com/repos/andrei-zededa/terraform-provider-zedamigo/releases/latest"	\
	| grep -E '^[[:space:]]+"name":[[:space:]]+"v[[:digit:]]+\.[[:digit:]]+\.[[:digit:]]+",$'		\
	| head -n1												\
	| sed -E 's/^[[:space:]]+"name":[[:space:]]+"v([[:digit:]]+\.[[:digit:]]+\.[[:digit:]]+)",$/\1/g')";

$_curl -o "$INSTALL_PATH/terraform-provider-zedamigo_${version}_linux_amd64.zip"	\
	"https://github.com/andrei-zededa/terraform-provider-zedamigo/releases/download/v${version}/terraform-provider-zedamigo_${version}_linux_amd64.zip";

_tf="$(command -v opentofu || command -v terraform || echo "")";

[ -z "$_tf" ] && exit 0;

TEMP_DIR="$(mktemp -d)";
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
