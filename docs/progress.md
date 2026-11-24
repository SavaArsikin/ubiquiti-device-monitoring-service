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

**Current state:** The monitor bootstrap, file-only configuration and logging
are implemented. The first stage remains open for Postgres and simulator
configuration examples and secret ignore rules.

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
- Targeted config and logger tests passed after the latest changes. The earlier
  full `go test ./... -count=1`, `go vet ./...`, and monitor startup passed.
  Dependency metadata was tidied; the unused `test-setup` recipe is commented
  out. `make test-file` runs the containing package, not only the named file.
- **Next:** add Postgres configuration/example and secret ignore rule alongside
  persistence setup, then migrations, inventory and snapshot/history storage.
  Add simulator configuration/example and ignore rule with the simulator.

When resuming, record the completed stage, decisions and unresolved questions,
verification performed, and the next concrete step here. Do not record secrets.
