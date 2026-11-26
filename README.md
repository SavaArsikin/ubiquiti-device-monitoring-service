# Ubiquiti device monitoring service

A Go proof of concept for monitoring a device inventory, with PostgreSQL
storage and simulated devices. The device protocols and checksum are
provisional: this project does **not** claim compatibility with Ubiquiti
hardware.

The monitor currently initializes configuration and logging, connects to
PostgreSQL, applies migrations, and exits. Its REST/gRPC APIs and polling loop
are not implemented yet; the simulators can be exercised independently.

## Components

- **Monitor:** configuration, logging and database startup; an inventory
  service and repositories for devices, current status and status history.
- **PostgreSQL:** local persistent storage with embedded Goose SQL migrations.
- **REST device simulator:** independent health discovery (`/health`) and
  diagnostics (`/diagnostics`) behaviors, including healthy, slow, flaky,
  error and incomplete responses. Its external checksum stub tests process
  wiring, not a real checksum algorithm. See [demo scenarios](demo/README.md).
- **Tests:** service unit tests use stubs and sqlmock; database and repository
  tests use a prepared local test database.

## Prerequisites

- [Go](https://go.dev/doc/install), version 1.27.2 or newer as required by
  `go.mod`, plus Git, Make and curl.
- [Podman](https://podman.io/docs/installation) and a Compose provider such as
  [podman-compose](https://github.com/containers/podman-compose#installation).
  On macOS, initialize and start the Podman machine with `podman machine init`
  and `podman machine start`. Alternatively install
  [Docker Desktop](https://docs.docker.com/get-started/get-docker/) with Compose
  and replace `podman compose` below with `docker compose`.
- Registry access to download the PostgreSQL, Go and Alpine base images.
  The simulator image builds Go dependencies from the included `vendor/`
  directory; vendoring does not replace the base images.

### Why vendor dependencies?

`vendor/` contains the Go dependency source used by the project, included
license files, and `vendor/modules.txt` with module version metadata. We
intend to commit it alongside `go.mod` and `go.sum`: the simulator image builds
with `-mod=vendor`, making its dependency inputs reproducible without reaching
a Go module proxy during the build. This also avoids the corporate TLS proxy
certificate failure encountered during `go mod download`, without disabling
TLS verification or shipping a corporate CA.

Vendoring increases repository size. It does not make the entire setup offline:
base images still require network access and registry trust, and native builds
require an installed Go toolchain. After dependency changes, run `go mod tidy`
then `go mod vendor` in a trusted environment and commit the matching `go.mod`,
`go.sum`, and `vendor/` changes together; do not hand-edit vendored files.

## Setup and run

Run commands from the repository root. Start with a fresh clone:

```sh
git clone https://github.com/SavaArsikin/ubiquiti-device-monitoring-service.git
cd ubiquiti-device-monitoring-service
cp -n .env.monitor.example .env.monitor
```

These instructions describe the current working tree; a fresh clone needs the
branch containing these changes after they are committed and published.

Edit `.env.monitor` for your local setup. The included PostgreSQL Compose
service and test helper use local demo credentials; they are not suitable for
production. Do not commit real credentials. The copy command does not overwrite
an existing file. The monitor reads this file directly, not the ambient
environment.

Start PostgreSQL and both prepared simulator instances in the foreground:

```sh
podman compose -f compose.yaml -f compose.simulators.yaml up --build
```

Leave it running for live logs; Ctrl+C stops the foreground stack. For a
detached stack, add `-d` to the command and follow logs with:

```sh
podman compose -f compose.yaml -f compose.simulators.yaml logs -f
```

In another terminal, wait until PostgreSQL is healthy:

```sh
podman compose -f compose.yaml -f compose.simulators.yaml ps
make db-setup
```

`db-setup` ensures the main database exists and prepares and migrates the
dedicated test database. It requires the demo PostgreSQL user to be able to
create databases. The test helper uses its own fixed local test configuration,
so changing the Compose database credentials also requires updating the test
configuration. Existing named volumes retain their database and credentials.

To initialize the monitor and migrate the main database:

```sh
go run ./cmd/monitor
```

It exits after database initialization; it does not yet serve requests.

### Try the simulators

The Compose files mount the complete `demo/simulator-healthy.env` and
`demo/simulator-flaky.env` files. These use the checksum executable built into
the image; `.env.simulator.example` is not a complete runnable simulator
configuration. Check the healthy instance:

```sh
curl -i http://127.0.0.1:8081/health
curl -i http://127.0.0.1:8081/diagnostics
```

Both return HTTP 200; health advertises `/diagnostics`, and diagnostics returns
the fake versions, status and stub checksum. The flaky instance's health
endpoint returns 503 for its first two requests, then 200. Restart it to reset
its counter before trying:

```sh
podman compose -f compose.yaml -f compose.simulators.yaml restart simulator-flaky
curl -i http://127.0.0.1:8082/health
curl -i http://127.0.0.1:8082/health
curl -i http://127.0.0.1:8082/health
```

These are simulator smoke checks, not tests of monitor discovery or polling.
For detached stacks, stop the services without deleting the database volume:

```sh
podman compose -f compose.yaml -f compose.simulators.yaml down
```

## Tests

After PostgreSQL is healthy and `make db-setup` has succeeded:

```sh
make test
```

This runs packages serially: database tests share and truncate tables in the
dedicated test database, so do not run the full suite with concurrent
`go test ./...` or alongside another run against that database.
`make test-verbose` provides verbose output. Service unit tests can run without
PostgreSQL using `go test ./internal/service`.

See the [implementation plan](docs/implementation-plan.md) and
[progress tracker](docs/progress.md) for what remains.
