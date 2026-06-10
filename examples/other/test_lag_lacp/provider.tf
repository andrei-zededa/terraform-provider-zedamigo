terraform {
  required_providers {
    zedamigo = {
      source = "localhost/andrei-zededa/zedamigo"
    }
  }
}

provider "zedamigo" {
  # Target host on which the zedamigo provider will execute commands and
  # create resources. ONLY `localhost` is currently supported. Optional and
  # if not specified it defaults to `localhost`.
  target = "localhost"

  # Creating a bond and TAP interfaces requires CAP_NET_ADMIN, so the provider
  # needs to run the `ip` commands via sudo.
  use_sudo = true
}
