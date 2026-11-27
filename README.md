# Ubiquiti device monitoring service

A simulator-only Go proof of concept for keeping a device inventory and
monitoring device health. Device discovery, diagnostics and the checksum
algorithm are provisional: the project does **not** claim compatibility with
real Ubiquiti devices or a real device checksum.

## Components and interfaces

- **Monitor:** stores inventory, discovers and polls active devices, and serves
  an authenticated REST API for inventory and current status. It runs locally
  as a Go process, not as a Compose service.
- **PostgreSQL:** stores inventory, the latest status snapshot and poll
  history. The monitor applies embedded Goose migrations at startup.
- **Device simulator:** provides HTTP discovery and REST diagnostics, with an
  optional gRPC diagnostics server. Configurable healthy, slow, flaky, error
  and incomplete responses exercise monitoring behavior. A separate one-shot
  checksum stub is invoked by the simulator; its demo SHA-256 checksum tests
  process wiring, not a hardware algorithm. See [simulator scenarios](demo/README.md).
- **Interfaces:** the monitor exposes REST only. Devices expose HTTP
  `GET /health` for discovery, then REST diagnostics or gRPC `GetDiagnostics`.
  gRPC is a device-facing transport, not a monitor API. These simulator
  protocols are not intended as production security interfaces.

## Monitoring behavior

Each device has a monitor-assigned UUID, a name and a health URL. Creation
also stores an initial status of `unknown`; it does not immediately probe the
device. The scheduler starts a sweep on monitor startup, then waits the
configured interval after each completed sweep. Creates, updates, archives
and restores affect polling on the next sweep. Work is bounded by the
configured concurrency, timeout and retry settings.

For each active device the monitor:

1. Requests the configured health URL over HTTP or HTTPS. The JSON discovery
   response can advertise `rest_diagnostics_path` and/or `grpc_target`. If
   both are present, gRPC is preferred; a failed advertised gRPC call does not
   fall back to REST. The REST path must be on the same device, and a gRPC
   target must use the same host as the health URL. The current gRPC
   connection is unencrypted and intended for the local simulator.
2. Requests diagnostics: hardware, software and firmware versions, the
   device-reported status and a checksum. The checksum stub receives
   diagnostics JSON without the checksum on standard input and returns a
   checksum on standard output. A missing version, status or checksum makes
   diagnostics incomplete.
3. Persists the latest snapshot and one history row per completed poll cycle,
   not per attempt. Diagnostic results from a partial response can be stored in
   history, but the snapshot retains its last complete diagnostics and
   `diagnostics_observed_at`. Thus those fields can be older than
   `last_checked_at`.

Reachability is separate from the device-reported status and diagnostic
completeness. Successful discovery marks the device `reachable` and resets
the consecutive reachability failure count, even if diagnostics or checksum
generation fails. A failed discovery cycle increments that count: the state is
`degraded` until `FAILURE_THRESHOLD` is reached, then `unreachable`.
Successful discovery after a failure restores `reachable`. The example
configuration uses a 10-second interval, a 2-second attempt timeout, one
retry after 250 milliseconds, and a threshold of three failed cycles. The
monitor retries incomplete attempts too; successful contact in an earlier
attempt is retained if a later retry fails discovery.

Archiving stops future polling and retains the device ID, latest snapshot and
history; restoring the same ID resumes polling on a later sweep. History
accumulates without automatic pruning. A repository delete-before operation
exists but is not called by the running service.

## Monitor REST API

The base URL is `http://127.0.0.1:8080` with the supplied example
configuration. Every route requires HTTP Basic Auth with `API_USERNAME` and
`API_PASSWORD` from `.env.monitor`. Basic Auth does not encrypt credentials:
keep the service on localhost or protect remote access with TLS. Requests with
bodies use JSON; successful responses are JSON except for archive's empty
response. IDs are UUIDs.

| Method | Path | Successful response and purpose |
| --- | --- | --- |
| `GET` | `/api/v1/devices` | `200`, array of active devices; `?state=active` is equivalent. |
| `GET` | `/api/v1/devices?state=archived` | `200`, array of archived devices. Other `state` values return `400`. |
| `POST` | `/api/v1/devices` | `201`, create a device and initial unknown status; `Location` identifies the device. |
| `GET` | `/api/v1/devices/{deviceID}` | `200`, get an active or archived device. |
| `PUT` | `/api/v1/devices/{deviceID}` | `200`, fully replace an active device's editable fields. |
| `DELETE` | `/api/v1/devices/{deviceID}` | `204`, soft-archive an active device; no response body. |
| `POST` | `/api/v1/devices/{deviceID}/restore` | `200`, restore an archived device with the same ID. |
| `GET` | `/api/v1/status` | `200`, array of latest snapshots for active devices. |
| `GET` | `/api/v1/devices/{deviceID}/status` | `200`, latest snapshot for a device, including an archived device. |

### Device request and response

Both `POST` and `PUT` require a non-empty name and an HTTP or HTTPS health URL
with a host. Leading and trailing whitespace is trimmed. `PUT` is a full
replacement, not a partial update:

```json
{"name":"Healthy simulator","health_url":"http://127.0.0.1:8081/health"}
```

Example device response (timestamps and ID vary):

```json
{
  "id": "b9e86469-1b9f-45c5-9ec8-f9e2bb530b71",
  "name": "Healthy simulator",
  "health_url": "http://127.0.0.1:8081/health",
  "created_at": "2026-01-01T12:00:00Z",
  "updated_at": "2026-01-01T12:00:00Z",
  "archived_at": null
}
```

### Status response

Immediately after creation the snapshot has the following shape. Optional
fields are `null` until information is available:

```json
{
  "device_id": "b9e86469-1b9f-45c5-9ec8-f9e2bb530b71",
  "reachability_status": "unknown",
  "consecutive_failures": 0,
  "diagnostics_complete": false,
  "last_checked_at": null,
  "last_reachable_at": null,
  "last_failure_category": null,
  "device_reported_status": null,
  "hardware_version": null,
  "software_version": null,
  "firmware_version": null,
  "checksum": null,
  "diagnostics_observed_at": null
}
```

`reachability_status` is `unknown`, `reachable`, `degraded` or `unreachable`.
`last_checked_at` is the latest completed poll; `last_reachable_at` is the
latest successful discovery. `last_failure_category` can identify a timeout,
discovery or diagnostics failure, unsupported protocol, invalid diagnostics
or unavailable checksum. A reachable device can still have incomplete
diagnostics. The API returns current snapshots, not the history rows.

### Errors

Application errors have the form
`{"error":{"code":"invalid_request","message":"..."}}`.

| HTTP status | Typical cause |
| --- | --- |
| `400` | Invalid UUID, missing/invalid device fields, invalid JSON or unknown JSON field, or invalid `state`. |
| `401` | Missing or invalid Basic Auth; includes a `WWW-Authenticate` challenge. |
| `404` | Unknown route, device or status; also updating/archiving an already archived device or restoring an active device. |
| `405` | Method not supported on a route; includes an `Allow` header. |
| `413` | Request body exceeds 1 MiB. |
| `500` | Internal service failure. |

## Install and configure

### Prerequisites

- [Git](https://git-scm.com/install/), [Go](https://go.dev/doc/install) at the
  version required by `go.mod` (currently 1.27.2 or newer), Make and curl.
- [Docker Desktop](https://docs.docker.com/get-started/get-docker/) with
  Docker Compose. Alternatively, install
  [Podman](https://podman.io/docs/installation) with a compatible Compose
  provider such as
  [podman-compose](https://github.com/containers/podman-compose#installation).
  On macOS with Podman, run `podman machine init` once and
  `podman machine start`; use `podman compose` in place of `docker compose`
  in the commands below.
- Access to a container registry for the PostgreSQL, Go and Alpine images.

Clone the repository and prepare monitor configuration:

```sh
git clone https://github.com/SavaArsikin/ubiquiti-device-monitoring-service.git
cd ubiquiti-device-monitoring-service
cp -n .env.monitor.example .env.monitor
```

These instructions describe the current working tree; a fresh clone must
contain these changes after they are committed and published. Edit
`.env.monitor` for the local setup. All required settings are read directly
from this file, not inherited from the shell; missing or invalid values stop
startup. The example uses local demo database and API credentials. Do not use
them in production or commit real credentials. `cp -n` does not replace an
existing configuration.

The example's `DATABASE_URL` matches the local Compose PostgreSQL service.
It also configures `HTTP_ADDR`, API credentials, logging, connection-pool
settings, polling interval, timeouts, retries, failure threshold and worker
count. The simulator instances use the separate files in `demo/`.

### Dependency modes and `vendor/`

`go.mod` and `go.sum` describe and verify the project's module dependencies.
`vendor/` contains their Go source, included license files, and
`vendor/modules.txt` version metadata. Keeping these together supports builds
without downloading Go modules and avoids the corporate TLS proxy certificate
failure previously encountered during `go mod download`, without disabling
TLS verification or distributing a corporate CA. The tradeoff is a larger
repository.

Choose **one** mode in each terminal before running native Go or Make commands:

```sh
# Use the Go module cache/proxy instead of vendor; network and TLS trust may be needed.
export GOFLAGS=-mod=mod
go mod download
```

If module downloading fails because of a proxy certificate, use the matching
vendored dependencies instead:

```sh
export GOFLAGS=-mod=vendor
```

Module mode does not bypass TLS problems; fix proxy trust when using it. The
simulator image independently always builds with `-mod=vendor`, regardless of
the shell's `GOFLAGS`. Vendoring does not supply the Go toolchain or container
base images, and image pulls still need registry access and trust. After a
dependency change, run `go mod tidy` and `go mod vendor` in a trusted
environment and commit matching `go.mod`, `go.sum` and `vendor/` changes;
do not hand-edit vendored files.

## Run from an empty database

1. From the repository root, start PostgreSQL and the three simulator
   instances:

   ```sh
   docker compose -f compose.yaml -f compose.simulators.yaml up --build
   ```

   Leave this terminal open for logs. The stack publishes PostgreSQL on
   `127.0.0.1:5432`, healthy REST discovery on `127.0.0.1:8081`, flaky REST
   discovery on `127.0.0.1:8082`, and gRPC simulator discovery on
   `127.0.0.1:8083` with diagnostics on `127.0.0.1:9091`. No devices are
   automatically registered with the monitor.

2. In another terminal, choose the same dependency mode as above and wait for
   PostgreSQL to be healthy:

   ```sh
   docker compose -f compose.yaml -f compose.simulators.yaml ps
   make db-setup
   ```

   `db-setup` ensures the configured main database exists and creates and
   migrates a separate local test database. The example PostgreSQL user needs
   permission to create databases. The test helper uses fixed local test
   credentials; changing Compose credentials also requires updating the test
   configuration. Existing named volumes retain their data and credentials.

3. Start the monitor in that terminal:

   ```sh
   go run ./cmd/monitor
   ```

   It loads `.env.monitor`, connects to PostgreSQL, migrates the main database,
   starts the REST server and begins polling. With an empty inventory,
   `GET /api/v1/devices` and `GET /api/v1/status` both return `[]`.

4. In a third terminal, use the API. Replace the sample credentials with your
   configured values; `curl -u demo` prompts for the password without putting
   it in the command. Create the healthy simulator and note the returned
   `id`:

   ```sh
   curl -u demo http://127.0.0.1:8080/api/v1/devices
   curl -u demo -i -X POST http://127.0.0.1:8080/api/v1/devices \
     -H 'Content-Type: application/json' \
     -d '{"name":"Healthy simulator","health_url":"http://127.0.0.1:8081/health"}'
   ```

   Set `DEVICE_ID` to the returned UUID, then inspect its status after the next
   scheduler sweep:

   ```sh
   DEVICE_ID='<returned UUID>'
   curl -u demo "http://127.0.0.1:8080/api/v1/devices/$DEVICE_ID/status"
   curl -u demo http://127.0.0.1:8080/api/v1/status
   ```

   The initial snapshot may be `unknown`; once successfully polled it becomes
   `reachable`, with complete diagnostics and the simulator's versions and
   checksum. To exercise gRPC, create a second device with health URL
   `http://127.0.0.1:8083/health`; its discovery advertises both transports,
   so the monitor uses gRPC. The supplied target uses `127.0.0.1`, which
   matches the health URL host when the monitor runs on the host.

5. To modify or retire the first device:

   ```sh
   curl -u demo -X PUT "http://127.0.0.1:8080/api/v1/devices/$DEVICE_ID" \
     -H 'Content-Type: application/json' \
     -d '{"name":"Renamed simulator","health_url":"http://127.0.0.1:8081/health"}'
   curl -u demo -X DELETE "http://127.0.0.1:8080/api/v1/devices/$DEVICE_ID"
   curl -u demo 'http://127.0.0.1:8080/api/v1/devices?state=archived'
   curl -u demo -X POST "http://127.0.0.1:8080/api/v1/devices/$DEVICE_ID/restore"
   ```

Press Ctrl+C in the monitor and Compose terminals to stop them. To run Compose
in the background, add `-d` to the `up --build` command, then use
`docker compose -f compose.yaml -f compose.simulators.yaml logs -f` for logs
and `docker compose -f compose.yaml -f compose.simulators.yaml down` to stop.
Stopping does not delete the named database volume.

## Tests

After PostgreSQL is healthy, `make db-setup` has succeeded, and a dependency
mode has been selected:

```sh
make test
```

The suite runs packages serially because database tests share and truncate
tables in the dedicated test database. Do not run it concurrently with
another suite against that database. `make test-verbose` prints individual
results; `go test ./internal/service` runs service unit tests without
PostgreSQL. REST route tests and REST/gRPC simulator smoke checks have been
reported passing; this is not a claim that a full automated live
monitor-to-device smoke test has run.
