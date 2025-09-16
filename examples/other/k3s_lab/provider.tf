terraform {
  required_providers {
    zedamigo = {
      source = "localhost/andrei-zededa/zedamigo"
    }

    random = {
      source  = "hashicorp/random"
      version = "3.7.2"
    }

    null = {
      source  = "hashicorp/null"
      version = "3.2.4"
    }

    # external = {
    #   source  = "hashicorp/external"
    #   version = "2.3.5"
    # }
  }
}

provider "zedamigo" {
  # Target host on which the zedamigo provider will execute commands and
  # create resources. ONLY `localhost` is currently supported. Optional and
  # if not specified it defaults to `localhost`.
  target = "localhost"

  # The provider lib directory, where all disk images and other files are
  # created on `target`. Optional and if not specified it defaults to
  # `$XDG_STATE_HOME/zedamigo/`, e.g. `$HOME/.local/state/zedamigo/`.
  # lib_path = ""

  use_sudo = true
}
