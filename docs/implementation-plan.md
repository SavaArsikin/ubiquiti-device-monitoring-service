# Implementation plan

This is a two-day, simulator-only Go proof of concept. It does not claim
compatibility with Ubiquiti hardware. The repository started as a minimal
scaffold without established health, device, protocol, server-library, or
checksum contracts. Define provisional fake-device contracts as the work
proceeds and keep device and checksum adapters replaceable.

See [progress.md](progress.md) for the current stage and handoff notes.

## Architecture and decisions

- Use `cmd/monitor`, `cmd/simulator`, and `cmd/checksumstub`, with internal
  packages for config, monitoring, persistence, APIs, device clients, and
  checksum execution. REST handlers use the standard-library `net/http` mux.
  Device-diagnostics protobuf definitions are under
  `internal/deviceprotocol/grpc/v1/`; Goose migrations live in
  `internal/database/migrations/` and are embedded by the database package.
- Use one shared application/service layer. REST clients use standard
  `net/http`; device diagnostics support REST and gRPC. The monitor itself
  exposes REST only; gRPC is a device-diagnostics transport.
- Use `database/sql` with the pgx stdlib driver, explicit SQL, and versioned
  Goose SQL migrations; no ORM.
- Read monitor and simulator configuration from their own env files, not
  inherited ambient environment variables. Config structs use `env` tags
  with `github.com/caarlos0/env/v11`; `godotenv.Read` supplies the map to
  `env.ParseWithOptions`. Unknown keys are ignored. Validate semantics such as
  positive durations and valid addresses. Required values must be present and
  valid, with no runtime defaults; simulator `CHECKSUM_ARGS` may be empty.
  Initialize global monitor configuration before starting goroutines; `Init`
  and `Get` return values, not mutable references.
  The monitor env file contains both monitor and database settings; the
  simulator has a separate env file. Never put real credentials in
  documentation.

## API decisions and implementation status

- **Confirmed REST contract:** Basic Auth uses credentials from the monitor
  configuration. `GET /api/v1/devices` lists active inventory by default;
  `GET /api/v1/devices?state=archived` lists archived inventory. `POST` to the
  collection creates a device. `GET /api/v1/devices/{deviceID}` reads one;
  `PUT` fully replaces its editable fields; `DELETE` soft-archives it.
  `POST /api/v1/devices/{deviceID}/restore` restores the same identity.
  `GET /api/v1/status` lists active-device statuses and
  `GET /api/v1/devices/{deviceID}/status` reads one status.
- `cmd/monitor` mounts and serves these REST routes with configured Basic Auth.
  REST route tests are present; the user reports they pass. No full live
  monitor/API smoke test is claimed here.
- **Next-sweep semantics:** inventory changes do not trigger immediate probes.
  The scheduler reads active inventory at the start of each sweep, so creates,
  updates, restores, and archives affect monitoring on the next sweep.
- Listen on localhost by default, with configurable addresses. Compose
  published ports bind to localhost. Basic Auth is not encryption: remote
  access requires TLS.
- Inventory stores monitor-assigned device IDs, display names, and editable
  health URLs. The ID, not the health URL, identifies a device; restoring by
  ID preserves its identity and associated history. Inventory changes are
  observed by the next scheduler sweep, not by an immediate probe.
- Every fake device serves HTTP `GET /health` for capability discovery,
  regardless of its diagnostics protocol. Its JSON response advertises a REST
  diagnostics path and/or gRPC target. Diagnostics include hardware, software,
  firmware, device-reported status, and checksum. Device diagnostics support
  both REST and gRPC; prefer gRPC when both transports are advertised.
- The Go simulator invokes a one-shot checksum process with diagnostics JSON
  excluding the checksum on stdin and `{"checksum":"..."}` on stdout, bounded
  by context. The stub validates wiring and lifecycle only, not a real checksum
  algorithm. Generator errors mean incomplete diagnostics/checksum
  unavailable, not an unreachable device.

## Polling and data

- Configure a 10-second polling interval in the example env file, a 2-second
  attempt timeout, one short retry, and a threshold of three failed cycles.
  Mark the first two failed cycles degraded and the third unreachable; a
  successful recovery resets the failure count. The current scheduler runs a
  sweep immediately on startup, then waits the configured interval after the
  sweep completes. Inventory changes are picked up by the next sweep, not
  probed immediately.
- Use a bounded worker pool targeting approximately 50 devices. Keep
  reachability, device-reported health, and diagnostics completeness separate.
- Persist inventory and the current snapshot, plus one history row per completed
  poll cycle, not per retry. History currently accumulates: there is no
  automatic pruning or retention setting, and the repository's delete-before
  operation is unused. Archiving stops polling while retaining the latest
  snapshot and history.

## Logs and local lifecycle

- Use `log/slog` with tint for human-readable terminal logs and its default color
  behavior; color is not configurable. Configure `LOG_FORMAT` as `local` or
  `json` and `LOG_LEVEL` as `debug`, `info`, `warn`, or `error`; JSON uses the
  standard JSON handler. Log lifecycle, retries, failures, recovery/state
  transitions, and checksum errors as structured events, without per-healthy-
  poll noise or credentials.
- Compose currently runs Postgres plus healthy, flaky and gRPC device
  simulators; the checksum stub is packaged inside each simulator image. It
  does not run the monitor. Keep Go service code OS-neutral; each binary must
  match its target OS.
- Service tests use stubs and sqlmock; database/repository tests require the
  prepared PostgreSQL test database and share/truncate tables, so run the
  complete suite serially with `make test`. REST route tests exist; automated
  simulator coverage and a full live monitor/API smoke test remain. Document
  local run, configuration, credentials handling, and reset in the README.

## Staged sequence and checkpoints

### Day 1: foundation and vertical slice

1. **Bootstrap and configuration — implemented:** monitor and simulator
   configuration, logging, examples, validation, and monitor startup are in
   place. Database settings are part of `.env.monitor`; a separate Postgres env
   file was not adopted.
2. **Persistence and shared services — core implemented:** Goose migrations,
   database setup, inventory/current-status/history repositories, service
   validation, archive/restore and transactional initial status are implemented.
   The REST probe client, monitoring service and bounded scheduler are also in
   place. History accumulates; the delete-before repository method is
   currently unused.
   **Checkpoint:** inventory and status/history persist; archive/restore keeps
   the device ID. User has reported service/repository tests passing; those
   results were not independently rerun for this plan update.
3. **Simulator and device probing — implemented:** REST and gRPC simulators,
   configurable endpoint behaviors, checksum stub, REST discovery, both
   diagnostics clients, protocol selection and monitoring poll integration
   exist. The checksum is a demo SHA-256, not a hardware algorithm.

### Day 2: complete behavior and local verification

4. **Monitor REST API — implemented:** monitor startup wires the handlers and
   configured Basic Auth. User reports route tests pass. Use `PUT` for full
   replacement, `DELETE` for soft archive,
   `POST /api/v1/devices/{deviceID}/restore` for restore, and
   `?state=archived` for archived listing. All-status and per-device status
   reads are part of REST.
5. **Next-sweep monitoring and device gRPC — implemented:** inventory changes
   are picked up by the next scheduler sweep. Device gRPC diagnostics are
   supported and preferred when advertised; the monitor exposes REST only.
6. **Remaining integration — pending:** include the monitor in Compose and add
   automated simulator/live end-to-end coverage.
   **Checkpoint:** documented end-to-end checks cover the monitor and both
   device diagnostic protocols.

The sequence is a target, not a claim that the work is complete. REST method
and path choices above are settled. REST and gRPC simulator smoke checks are
user-reported and were not independently run for this plan update.

## Out of scope

UI, metrics platforms, alerting, production TLS setup, large-scale queueing,
real hardware interoperability, and validation of a real checksum binary.
