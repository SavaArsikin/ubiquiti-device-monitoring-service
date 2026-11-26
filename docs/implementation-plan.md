# Implementation plan

This is a two-day, simulator-only Go proof of concept. It does not claim
compatibility with Ubiquiti hardware. The repository started as a minimal
scaffold without established health, device, protocol, server-library, or
checksum contracts. Define provisional fake-device contracts as the work
proceeds and keep device and checksum adapters replaceable.

See [progress.md](progress.md) for the current stage and handoff notes.

## Architecture and decisions

- Use `cmd/monitor` and `cmd/device-sim`, with internal packages for config,
  monitoring, persistence, APIs, device clients, and checksum execution.
  Keep protobuf definitions in `api/`; Goose migrations currently live in
  `internal/database/migrations/` and are embedded by the database package.
- Use one shared application/service layer. REST uses chi and standard
  `net/http` clients. gRPC uses `google.golang.org/grpc`, with protobuf code
  generated using Buf.
- Use `database/sql` with the pgx stdlib driver, explicit SQL, and versioned
  Goose SQL migrations; no ORM.
- Read configuration directly from separate monitor, Postgres, and simulator
  env files, not inherited ambient environment variables. Annotate config
  structs with `env` tags using `github.com/caarlos0/env/v11`. Read the
  component file with `godotenv.Read(path)` and pass the resulting map to
  `env.ParseWithOptions(&cfg, env.Options{Environment: values, RequiredIfNoDef: true})`;
  do not call `env.Parse` or read ambient OS environment. Unknown keys are
  ignored. Validate semantics such as positive durations and valid addresses.
  Every required key must be present, non-empty, and valid; the application must
  not supply defaults for missing keys. Initialize global configuration before
  starting goroutines; `Init` and `Get` return values, not mutable references.
  Track complete `.env.*.example` templates and ignore real secret files.
  Finalize exact keys
  and validation in the config stage. Example values may express planned
  defaults, but are still required at runtime. Never put real credentials in
  documentation.

## Provisional contracts

- The monitor exposes Basic Auth-protected REST inventory CRUD at
  `/api/v1/devices` using POST, GET, PATCH, and DELETE, plus
  `GET /api/v1/status`. DELETE archives rather than hard-deletes. Provide
  explicit restore by existing device ID and a way to list archived devices;
  their API contracts are not implemented yet. Define individual-resource
  paths, payloads, and error responses during API work.
- A read-only gRPC `ListStatuses` returns the same status model as REST. Both
  APIs use shared credentials from the monitor env file; gRPC uses equivalent
  metadata and an interceptor. Never log credentials.
- Listen on localhost by default, with configurable addresses. Compose
  published ports bind to localhost. Basic Auth is not encryption: remote
  access requires TLS.
- Inventory stores monitor-assigned device IDs, display names, and editable
  health URLs. The ID, not the health URL, identifies a device; restoring by
  ID preserves its identity and associated history. A valid create or update
  triggers an immediate probe.
- Every fake device serves HTTP `GET /health` for capability discovery,
  regardless of its diagnostics protocol. Its JSON response advertises a REST
  diagnostics path and/or gRPC target; prefer gRPC when both are available.
  Diagnostics include hardware, software, firmware, device-reported status,
  and checksum. Finalize field names and wire formats during implementation.
- The Go simulator invokes a one-shot checksum process with diagnostics JSON
  excluding the checksum on stdin and `{"checksum":"..."}` on stdout, bounded
  by context. The stub validates wiring and lifecycle only, not a real checksum
  algorithm. Generator errors mean incomplete diagnostics/checksum
  unavailable, not an unreachable device.

## Polling and data

- Configure a 10-second polling interval in the example env file, a 2-second
  attempt timeout, one short retry, and a threshold of three failed cycles.
  Mark the first two failed cycles degraded and the third unreachable; a
  successful recovery resets the failure count. Finalize the retry delay and
  configuration keys during implementation.
- Use a bounded worker pool targeting approximately 50 devices. Keep
  reachability, device-reported health, and diagnostics completeness separate.
- Persist inventory and the current snapshot, plus one history row per completed
  poll cycle, not per retry. Configure history retention with a planned 24-hour
  example value, required at runtime, and periodically prune only old history.
  Keep latest snapshots and archived identities. Archiving stops polling while
  retaining the latest snapshot and history subject to normal retention.

## Logs and local lifecycle

- Use `log/slog` with tint for human-readable terminal logs and its default color
  behavior; color is not configurable. Configure `LOG_FORMAT` as `local` or
  `json` and `LOG_LEVEL` as `debug`, `info`, `warn`, or `error`; JSON uses the
  standard JSON handler. Log lifecycle, retries, failures, recovery/state
  transitions, and checksum errors as structured events, without per-healthy-
  poll noise or credentials.
- Compose runs Postgres, monitor, fake REST/gRPC devices, and the checksum
  stub with separate component env files. Seed healthy, flaky, and offline
  examples. Keep Go service code OS-neutral; each binary must match its target
  OS.
- Keep unit tests fast and hardware-free. Current database and repository tests
  require a locally prepared PostgreSQL test database; separate the integration
  test path from the hardware-free suite. Extend integration coverage to
  persistence, API/auth, protocol discovery, retry/recovery, and checksum
  failure. Document local run, configuration, credentials handling, and reset
  in the README.

## Staged sequence and checkpoints

### Day 1: foundation and vertical slice

1. **Scaffold and configuration:** create the module and layout, define required
   component-specific env keys and complete examples, validate configuration,
   ignore real secrets, and set up logging.
   **Monitor checkpoint:** the monitor starts with a valid file and rejects
   missing, empty, or semantically invalid required configuration without
   consulting ambient environment. Unknown keys are ignored.
   **Remaining:** add separate Postgres and simulator env examples and secret
   ignore rules as those components are introduced.
2. **Persistence and shared service:** add Postgres, Goose migrations, inventory,
   snapshot/history storage, archival and restore, and the shared application
   layer. The devices migration and repository are in place; snapshots,
   history, and the service layer remain.
   **Checkpoint:** inventory and snapshots survive restart; archive and restore
   preserve identity and data.
3. **Simulator and first probe:** define provisional discovery/diagnostic and
   checksum contracts; build the simulator, checksum stub, and initial
   health/diagnostics path.
   **Checkpoint:** a simulated device can be discovered and probed, including
   checksum success and a distinct checksum failure.

### Day 2: complete behavior and local verification

4. **APIs and protocols:** add REST CRUD/status, authenticated gRPC
   `ListStatuses`, REST and gRPC diagnostic clients, protocol preference, and
   immediate probes after valid creates/updates.
   **Checkpoint:** both authenticated APIs agree on status; discovery chooses
   gRPC when both protocols are advertised.
5. **Polling and retention:** add bounded workers, timeout/retry, state
   transitions and recovery, cycle history, archival behavior, and pruning.
   **Checkpoint:** failed cycles degrade then become unreachable, recovery
   resets the count, retries do not create extra history, and pruning preserves
   current snapshots and identities.
6. **Compose, tests, and handoff:** wire the local lifecycle and seeded
   examples, add focused unit and Compose-backed integration tests, and finish
   the README.
   **Checkpoint:** the hardware-free test suite and documented integration path
   cover the agreed behaviors and a developer can run and reset the stack.

The sequence is a target, not a claim that the work is complete. Resolve exact
env keys, fake-device payloads, resource paths, gRPC schemas, retry delay, and
seed mechanics in their respective stages; record decisions in the progress
tracker.

## Out of scope

UI, metrics platforms, alerting, production TLS setup, large-scale queueing,
real hardware interoperability, and validation of a real checksum binary.
