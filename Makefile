terraform-provider-zedamigo.schema.json: internal/provider
	tofu -chdir=examples/provider providers schema -json | jq '.provider_schemas |= (. + {"zedamigo": ."registry.opentofu.org/andrei-zededa/zedamigo"} | del(."registry.opentofu.org/andrei-zededa/zedamigo"))' > terraform-provider-zedamigo.schema.json

docs: terraform-provider-zedamigo.schema.json
	go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs generate --providers-schema ./terraform-provider-zedamigo.schema.json --provider-name zedamigo --examples-dir ./examples/
	cp docs/index.md docs/README.md

# --- Local dev install ---
#
# Build the provider for the host platform and install it into the local
# terraform/tofu plugin mirror so it can be used without a published release.
# The build injects main.version=dev and main.srcDir=$(CURDIR); the "dev"
# version makes the provider, when driving a REMOTE target, cross-compile a
# target-arch binary from this source checkout and upload it (see
# bootstrapDevBinary). Reference it in a config with:
#     source  = "localhost/andrei-zededa/zedamigo"
#     version = "$(DEV_VERSION)"
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
DEV_VERSION ?= 0.0.1
DEV_PLUGIN_DIR = $(HOME)/.terraform.d/plugins/localhost/andrei-zededa/zedamigo/$(DEV_VERSION)/$(GOOS)_$(GOARCH)

.PHONY: dev-install
dev-install:
	mkdir -p "$(DEV_PLUGIN_DIR)"
	CGO_ENABLED=0 go build -trimpath -tags "osusergo netgo" \
		-ldflags "-X main.version=dev -X 'main.srcDir=$(CURDIR)'" \
		-o "$(DEV_PLUGIN_DIR)/terraform-provider-zedamigo_v$(DEV_VERSION)" .
	@echo "Installed dev provider to $(DEV_PLUGIN_DIR)"
	@echo 'Use source = "localhost/andrei-zededa/zedamigo", version = "$(DEV_VERSION)" in required_providers.'
