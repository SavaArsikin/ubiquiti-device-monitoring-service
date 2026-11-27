# Implementation progress

Plan: [implementation-plan.md](implementation-plan.md)

- [x] Agree on the two-day, simulator-only Go PoC scope.
- [x] Bootstrap, logging and monitor/simulator configuration.
- [x] PostgreSQL migrations, inventory/current-status/history repositories, and
  shared inventory/monitoring services.
- [x] REST and gRPC device diagnostics, checksum stub, and bounded scheduler.
- [x] Basic-authenticated REST API for inventory and current status.
- [ ] Add monitor to Compose; automated simulator and full live end-to-end tests.

**Current state:** The monitor serves REST and polls devices. Device discovery
uses HTTP; diagnostics use REST or gRPC, preferring gRPC when advertised. The
Compose setup includes PostgreSQL and healthy, flaky, and gRPC simulator
instances. The monitor exposes REST only; gRPC is used for device diagnostics.
History accumulates without automatic pruning. The repository's delete-before
operation exists but is currently unused; no retention setting or pruning task
is planned.

## Decisions and handoff

- REST contract: Basic Auth; active inventory at `GET /api/v1/devices`,
  archived inventory at `GET /api/v1/devices?state=archived`, create with
  `POST`, full replacement with `PUT /api/v1/devices/{deviceID}`, soft archive
  with `DELETE`, and restore with `POST .../{deviceID}/restore`. Status reads
  are `GET /api/v1/status` and `GET /api/v1/devices/{deviceID}/status`.
- Inventory changes take effect on the next scheduler sweep, not via immediate
  probes. The scheduler starts a sweep on startup and delays the next sweep
  until the configured interval after completion.
- History rows are persisted indefinitely. The delete-before repository
  operation is unused; automatic pruning is not part of the plan.
- User-reported verification: service/repository tests pass, REST route tests
  pass, and REST plus gRPC simulator smoke checks succeed. These were not
  independently rerun for this tracker update; do not treat them as a full
  monitor-to-device end-to-end test.
- Database/repository tests require the prepared test database and share tables;
  run the full suite serially with `make test`. Service tests use stubs and
  sqlmock.
- **Next:** wire the monitor into Compose and add automated simulator and full
  end-to-end coverage.

When resuming, update the checkboxes and record the exact verification run.
Never put real credentials in this document.
