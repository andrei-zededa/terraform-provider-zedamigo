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

  use_sudo = true
}
