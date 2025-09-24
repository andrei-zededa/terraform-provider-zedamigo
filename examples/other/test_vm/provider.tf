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

  # The provider lib directory, where all disk images and other files are
  # created on `target`. Optional and if not specified it defaults to
  # `$XDG_STATE_HOME/zedamigo/`, e.g. `$HOME/.local/state/zedamigo/`.
  # lib_path = ""
}
