# Implementation progress

Plan: [implementation-plan.md](implementation-plan.md)

- [x] Agree on the simulator-only implementation plan.
- [ ] Day 1: Scaffold, configuration, and logging (monitor bootstrap done;
  other component env files and ignore rules remain).
- [ ] Day 1: Persistence and shared service.
- [ ] Day 1: Simulator and first probe.
- [ ] Day 2: APIs and diagnostic protocols.
- [ ] Day 2: Polling, recovery, history, and retention.
- [ ] Day 2: Compose, tests, and README handoff.

**Current state:** Monitor bootstrap, inventory service, persistence and a REST
simulator are implemented. The monitor currently initializes its database and
exits; APIs and polling are not connected.

## Update notes

- Configuration decision: use `github.com/caarlos0/env/v11` with struct `env`
  tags and parse only the map read by `godotenv.Read`. Required values have no
  code defaults or ambient fallback; unknown keys are ignored and semantic
  validation is explicit. `Init` and `Get` return configuration copies.
- The monitor example covers its required keys; `.env.monitor` is ignored.
  Logger configuration uses `LOG_FORMAT=local|json` and
  `LOG_LEVEL=debug|info|warn|error`; tint's default color behavior is not
  configurable.
- Shared test helpers initialize config. Config tests cover validation,
  missing file/key, no ambient fallback and value copies. Logger tests cover
  local and JSON output, level filtering and nested attributes, stripping ANSI
  codes before checking local output.
- PostgreSQL connection, local setup, embedded Goose migrations and repositories
  for inventory, current status and history are implemented. The inventory
  service supports create, get, active/archived lists, update, archive and
  restore; creation inserts inventory and an initial unknown status in one
  service-owned transaction. Business validation belongs to the service; the
  schema uses structural constraints. Health URLs are editable, not identity;
  restoration uses the existing ID. Restore and listing APIs remain pending.
- A REST simulator exposes discovery and diagnostics with independently
  configurable behaviors and a bounded one-shot external checksum stub. The
  REST probe client exists but is not wired to monitor polling. Compose runs
  PostgreSQL and healthy/flaky simulators; its simulator build uses vendored
  dependencies. The user reports healthy/flaky startup and HTTP checks work.
- The user reports service and repository tests pass. Service tests use static
  stubs and sqlmock; database/repository tests need the prepared test database
  and truncate shared tables, so use `make test` for serial packages. No full
  suite result is claimed for the latest changes. Automated simulator tests are
  still pending.
- **Next:** integrate discovery and polling with the service and repositories,
  add simulator tests, then implement the REST/gRPC APIs and gRPC simulator.
  Complete separate Postgres/simulator env examples and secret ignore rules,
  scheduling, retention and full Compose integration.

When resuming, record the completed stage, decisions and unresolved questions,
verification performed, and the next concrete step here. Do not record secrets.
