terraform {
  required_providers {
    zedamigo = {
      source = "andrei-zededa/zedamigo"
    }
  }
}

provider "zedamigo" {
  # Target host on which the zedamigo provider will execute commands and
  # create resources. Defaults to `localhost`. Optional.
  target = "localhost"

  # The provider lib directory, where all disk images and other files are
  # created on `target`. Optional and if not specified it defaults to
  # `$XDG_STATE_HOME/zedamigo/`, e.g. `$HOME/.local/state/zedamigo/`.
  # lib_path = ""

  # The default reservations path is /var/lib/zedamigo/reservations, which
  # typically requires root to write; use_sudo is not sufficient here since the
  # reservation script writes marker files directly. Either pre-create the tree
  # writable by the provider's user, or override `path` to a writable location.
  use_sudo = true
}
