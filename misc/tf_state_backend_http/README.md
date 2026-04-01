# Terraform HTTP State Backend

A minimal HTTP server that implements the
[Terraform HTTP backend](https://developer.hashicorp.com/terraform/language/backend/http)
protocol, storing `.tfstate` files on the local filesystem.
Intended for development and testing -- not for production use.

## Configuration

| Environment Variable    | Default                  | Description                |
|-------------------------|--------------------------|----------------------------|
| `TF_BACKEND_ADDR`      | `192.168.192.168:9000`   | Listen address (host:port) |
| `TF_BACKEND_STATE_DIR` | `./terraform_states`     | Directory for state files  |

## Build

```bash
cd misc/tf_state_backend_http
go build
```

Or run directly without building:

```bash
go run ./misc/tf_state_backend_http
```

## Run

Foreground:

```bash
./tf_state_backend_http
```

With custom settings:

```bash
TF_BACKEND_ADDR=0.0.0.0:8080 TF_BACKEND_STATE_DIR=/tmp/tf-states ./tf_state_backend_http
```

### Running as a background process

Start with stdout and stderr redirected to log files:

```bash
./tf_state_backend_http > tf-backend-stdout.log 2> tf-backend-stderr.log &
echo "PID: $!"
```

To stop it (the server handles SIGTERM gracefully):

```bash
kill $PID
```

## Terraform backend configuration

```hcl
terraform {
  backend "http" {
    address        = "http://192.168.192.168:9000/my-project"
    lock_address   = "http://192.168.192.168:9000/my-project"
    unlock_address = "http://192.168.192.168:9000/my-project"
  }
}
```

The URL path determines where the state file is stored on disk.
For example, `/my-project` maps to `<state_dir>/my-project` and
`/team/project` maps to `<state_dir>/team/project`
(subdirectories are created automatically).

## API

| Method      | Description                                              |
|-------------|----------------------------------------------------------|
| GET         | Read state. Returns 404 if the file does not exist yet.  |
| POST / PUT  | Write state. Creates subdirectories as needed.           |
| DELETE      | Remove state. Returns 200 even if already absent.        |
| LOCK        | Stubbed -- returns 200, no actual locking is performed.  |
| UNLOCK      | Stubbed -- returns 200, no actual locking is performed.  |

## Known limitations

- **No state locking.** LOCK and UNLOCK return 200 OK without performing
  any actual locking. Do not run concurrent Terraform operations against the
  same state path.
- **No authentication.** The server accepts `username` and `password` from
  Terraform's HTTP backend configuration but does not validate them.
- **No TLS.** Traffic is unencrypted.
- **Not designed for production.** Use a proper backend (S3, Consul,
  Terraform Cloud, etc.) for production state management.
