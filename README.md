# zedamigo - A terraform provider to manage EVE-OS edge-nodes running as QEMU VMs

```
mkdir -p "$HOME/git/github.com/andrei-zededa"
cd "$HOME/git/github.com/andrei-zededa"
git clone git@github.com:andrei-zededa/terraform-provider-zedamigo
cd terraform-provider-zedamigo/ 
go build
```

```
❯ cat terraform.tf
terraform {
  required_providers {
    zedamigo = {
      source  = "andrei-zededa/zedamigo"
      version = "0.1.0-dev"
    }

    zedcloud = {
      source  = "zededa/zedcloud"
      version = "2.4.0"
    }
  }
}

provider "zedamigo" {
  use_sudo = true
}
```

```
❯ cat "$HOME/.config/opentofu/tofurc" ## or "$HOME/.terraformrc"

provider_installation {
    dev_overrides {
      "andrei-zededa/zedamigo" = "/home/<<MY_USER>>/git/github.com/andrei-zededa/terraform-provider-zedamigo"
    }

    # For all other providers, install them directly from their origin provider
    # registries as normal. If you omit this, OpenTofu will _only_ use
    # the dev_overrides block, and so no other providers will be available.
    direct {}
 }
```
