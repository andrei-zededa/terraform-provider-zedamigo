terraform {
  required_providers {
    zedamigo = {
      source = "andrei-zededa/zedamigo"
    }
  }
}

provider "zedamigo" {
  # Target host on which the zedamigo provider will execute commands and create
  # resources. Defaults to `localhost` (everything runs on the machine running
  # the provider). Set it to a hostname or IP address to operate on a remote
  # host over SSH (configure the connection in the `ssh` block below). Optional.
  target = "localhost"

  # The provider lib directory, where all disk images and other files are
  # created on `target`. Optional and if not specified it defaults to
  # `$XDG_STATE_HOME/zedamigo/`, e.g. `$HOME/.local/state/zedamigo/`. For a
  # remote `target` the default is resolved from the remote host's environment.
  # lib_path = ""

  # Use `sudo` for running specific (but not all) commands that need to
  # be executed as the root user. Optional and if not specified it defaults
  # to `false`.
  # use_sudo = false

  # When `target` is a remote host, configure the SSH connection here. All
  # attributes are optional and each has a ZEDAMIGO_SSH_* environment fallback.
  # Provide at least one authentication method: password, private_key /
  # private_key_file, or use_agent.
  #
  # ssh {
  #   user             = "andrei"            # default: current local user
  #   port             = 22
  #   private_key_file = "~/.ssh/id_ed25519" # or: private_key = "<PEM>"
  #   # password       = "..."
  #   # use_agent      = true                # use $SSH_AUTH_SOCK
  #
  #   # Host key verification (fails closed): defaults to ~/.ssh/known_hosts.
  #   # known_hosts_file         = "~/.ssh/known_hosts"
  #   # host_key                 = "ssh-ed25519 AAAA..."
  #   # insecure_ignore_host_key = false     # INSECURE; dev/test only
  #
  #   # Path to the provider binary on the remote host (used by the self-invoked
  #   # daemons). If unset, it is bootstrapped via the install script pinned to
  #   # this provider's version (which fetches the binary for the remote arch).
  #   # remote_binary_path = ""
  # }
}
