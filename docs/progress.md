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

**Current state:** Monitor bootstrap, file-only configuration and logging are
implemented. Database setup and device inventory persistence are underway;
snapshots, history, and the shared service layer remain.

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
- PostgreSQL connection, embedded Goose devices migration, local setup, and a
  device repository are implemented. The monitor opens the database and applies
  migrations at startup. The repository supports create, get, list active,
  update, soft archive, and explicit restore by ID, with optional transactions.
  A health URL is editable and is not the device identity; restore retains the
  ID needed to associate future history. Listing archived devices and a restore
  API remain future work.
- The user reports that the restore tests pass. Database/repository tests
  require the prepared local test database; no full-suite result is claimed
  for the current changes. The earlier config/logger tests, full suite, vet,
  and bootstrap startup passed before the database work.
- **Next:** implement snapshot/history persistence and the shared service layer.
  Add Postgres and simulator env examples and secret ignore rules as those
  components are completed, and separate database-dependent tests from the
  hardware-free suite.

When resuming, record the completed stage, decisions and unresolved questions,
verification performed, and the next concrete step here. Do not record secrets.
