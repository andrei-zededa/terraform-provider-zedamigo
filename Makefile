terraform-provider-zedamigo.schema.json: internal/provider
	tofu -chdir=examples/provider providers schema -json | jq '.provider_schemas |= (. + {"zedamigo": ."registry.opentofu.org/andrei-zededa/zedamigo"} | del(."registry.opentofu.org/andrei-zededa/zedamigo"))' > terraform-provider-zedamigo.schema.json

docs: terraform-provider-zedamigo.schema.json
	go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs generate --providers-schema ./terraform-provider-zedamigo.schema.json --provider-name zedamigo --examples-dir ./examples/
	cp docs/index.md docs/README.md
