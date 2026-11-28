# Device simulator demo

The simulator provides HTTP health discovery and REST diagnostics, and one
Compose instance also serves diagnostics over gRPC. Its fake versions,
reported status and checksum are for exercising the monitor only: they do not
represent real Ubiquiti hardware or a device-compatible checksum algorithm.

## Start the demo

Install [Docker Desktop](https://docs.docker.com/get-started/get-docker/) with
Docker Compose. From the repository root, start the database and all four
simulator instances:

```sh
docker compose -f compose.yaml -f compose.simulators.yaml up --build
```

Leave the command running to see logs; Ctrl+C stops the foreground stack. In
another terminal, the instances are available at:

| Instance | Health/discovery URL | Diagnostics |
| --- | --- | --- |
| `simulator-healthy` | `http://127.0.0.1:8081/health` | REST `http://127.0.0.1:8081/diagnostics` |
| `simulator-flaky` | `http://127.0.0.1:8082/health` | REST `http://127.0.0.1:8082/diagnostics` |
| `simulator-slow` | `http://127.0.0.1:8084/health` | REST `http://127.0.0.1:8084/diagnostics` |
| `simulator-grpc` | `http://127.0.0.1:8083/health` | REST `http://127.0.0.1:8083/diagnostics` or gRPC `127.0.0.1:9091` |

Compose also starts PostgreSQL on `127.0.0.1:5432`. It does **not** start the
monitor or automatically register devices. Run the monitor separately using
the main [project setup](../README.md); for a monitor running on the host, add
the health URLs above to its REST API. The gRPC instance advertises both
diagnostic transports, so the monitor prefers gRPC. Its configured gRPC target
is `127.0.0.1:9091`, suitable for a monitor running on the host. A monitor in
another container would need a reachable service-name target instead.

To run with Docker in the background, add `-d` to the `up --build` command.
Then use:

```sh
docker compose -f compose.yaml -f compose.simulators.yaml ps
docker compose -f compose.yaml -f compose.simulators.yaml logs -f
docker compose -f compose.yaml -f compose.simulators.yaml down
```

Podman is an alternative. Install Podman and a compatible Compose provider
such as [podman-compose](https://github.com/containers/podman-compose#installation);
on macOS initialize and start the Podman machine with `podman machine init`
and `podman machine start`. Replace `docker compose` with `podman compose` in
the commands above. Stopping the stack does not delete the named PostgreSQL
volume.

## What the simulator returns

Every instance serves:

- `GET /health`: on success, returns HTTP 200 JSON advertising the REST
  diagnostics path and, when configured, a gRPC target. This discovery
  endpoint is available even for the instance that serves diagnostics over
  gRPC.
- `GET /diagnostics`: on success, returns HTTP 200 JSON containing
  `hardware_version`, `software_version`, `firmware_version`, `status`,
  `checksum`, and, for incomplete results, `failure_category`.
- gRPC `GetDiagnostics`: enabled only on `simulator-grpc`; returns the same
  diagnostics fields through the `DeviceDiagnostics` service.

The independent health and REST-diagnostics endpoints each keep their own
request counter. The gRPC diagnostics server has a separate counter.
`FAILURE_COUNT` controls how many initial responses are affected in `flaky`,
`slow`, and `error` modes for each endpoint or transport. `flaky` and `error`
fail those first requests, then succeed; `slow` delays those first requests,
then responds without a delay. A value of `0` means no failures or delay.
Restarting an instance resets all its counters.

`slow` waits for `RESPONSE_DELAY` on the first `FAILURE_COUNT` requests to the
endpoint or transport configured as slow, then responds normally without a
delay. The HTTP health, HTTP diagnostics, and gRPC diagnostics counters are
independent. Each request consumes a count, including one canceled while
waiting; if the client cancels or times out before the delay ends, that
request does not return a successful response. In `error` mode, HTTP endpoints
return the configured `ERROR_STATUS` for the first `FAILURE_COUNT` requests
and succeed afterward; gRPC diagnostics returns an `Unavailable` status for
its first counted requests and succeeds afterward.
`incomplete` is available only for diagnostics and still returns HTTP 200 or
a successful gRPC response, but with an empty firmware version and
`failure_category: "invalid_diagnostics"`. The checksum is not run for this
incomplete case.

For complete diagnostics, the simulator runs the configured checksum
executable as a one-shot child process. It passes JSON containing the fake
hardware/software/firmware versions and device-reported status (not a checksum)
to stdin, and expects `{"checksum":"..."}` JSON on stdout. The bundled
`checksumstub` returns a SHA-256 hash of canonicalized input. This demonstrates
process wiring only. If the process times out, exits unsuccessfully, emits
invalid JSON, produces an empty checksum, or exceeds the output limit, the
diagnostic endpoint still responds successfully with `checksum: null` and
`failure_category: "checksum_unavailable"`. This is a diagnostics failure,
not evidence that the device is unreachable.

## Simulator configuration

Compose mounts the four separate demo env files read-only as
`/app/simulator.env`. The simulator reads the file supplied by the
`-config` flag at startup; it does not read ambient shell environment values.
All listed common settings are required. The gRPC listener and advertised
target are optional but must be configured together. Use valid addresses,
positive durations, a nonnegative failure count and an HTTP error status from
400 through 599. `CHECKSUM_ARGS` is required but may be empty.

| Variable | Effect |
| --- | --- |
| `HTTP_ADDR` | Simulator HTTP listen address. Compose uses `0.0.0.0:8081` inside each container. The host mappings are `127.0.0.1:8081` for healthy, `:8082` for flaky, `:8083` for gRPC and `:8084` for slow. |
| `GRPC_LISTEN_ADDR` | Optional gRPC listen address. Set together with `GRPC_TARGET`; the gRPC instance listens on `0.0.0.0:9091`. |
| `GRPC_TARGET` | Optional target advertised by `/health`; must be a reachable host-and-port, not an unspecified address. The demo advertises `127.0.0.1:9091`. |
| `READ_HEADER_TIMEOUT` | Maximum time allowed for reading HTTP request headers. |
| `SHUTDOWN_TIMEOUT` | Grace period for HTTP and gRPC servers to shut down after a signal. Compose's container stop grace period is 15 seconds. |
| `HARDWARE_VERSION` | Fake hardware version returned by diagnostics. |
| `SOFTWARE_VERSION` | Fake software version returned by diagnostics. |
| `FIRMWARE_VERSION` | Fake firmware version. The incomplete diagnostics behavior overrides it with an empty value. |
| `DEVICE_STATUS` | Fake device-reported status; does not determine endpoint reachability. |
| `HEALTH_BEHAVIOR` | `healthy`, `slow`, `flaky`, or `error`, applied only to HTTP `/health`. |
| `DIAGNOSTICS_BEHAVIOR` | `healthy`, `slow`, `flaky`, `error`, or `incomplete`, applied to REST diagnostics and, when enabled, gRPC diagnostics. |
| `RESPONSE_DELAY` | Positive delay used only when the corresponding behavior is `slow`. |
| `FAILURE_COUNT` | Number of initial requests affected by `flaky`, `slow`, or `error`, per endpoint/transport. Flaky/error requests fail; slow requests wait before succeeding. Zero disables these initial failures/delays. Health, REST diagnostics and gRPC diagnostics have independent counters, reset when the simulator restarts. |
| `ERROR_STATUS` | HTTP status used by `error` and initial `flaky` HTTP responses; must be from 400 through 599. |
| `CHECKSUM_BINARY` | Required executable path. The image includes `/app/checksumstub`; the simulator fails to start if the executable cannot be found. |
| `CHECKSUM_ARGS` | Required comma-separated arguments to the executable; empty means no arguments. Use `--fail` with the included stub to simulate a process failure. |
| `CHECKSUM_TIMEOUT` | Positive maximum runtime for the checksum process. |

## Try the included instances

Healthy REST discovery and diagnostics each return HTTP 200:

```sh
curl -i http://127.0.0.1:8081/health
curl -i http://127.0.0.1:8081/diagnostics
```

The flaky instance returns 503 for its first two health requests, then 200.
Restart only that instance to reset its counters:

```sh
docker compose -f compose.yaml -f compose.simulators.yaml restart simulator-flaky
curl -i http://127.0.0.1:8082/health
curl -i http://127.0.0.1:8082/health
curl -i http://127.0.0.1:8082/health
```

The dedicated slow instance has `HEALTH_BEHAVIOR=slow`,
`FAILURE_COUNT=2`, and `RESPONSE_DELAY=3s`. Its first two health requests
each take about three seconds and return 200; the third responds immediately.
Observe the counter behavior directly:

```sh
curl -i http://127.0.0.1:8084/health
curl -i http://127.0.0.1:8084/health
curl -i http://127.0.0.1:8084/health
```

For monitor recovery, add `http://127.0.0.1:8084/health` as a device while
the monitor is running. With the example monitor settings, both attempts in
the first poll cycle time out (each has a two-second timeout and the simulator
waits three seconds), making the device `degraded`. The monitor waits its
configured polling interval after the completed sweep; on the next sweep the
simulator's slow-request count is exhausted and the device returns to
`reachable`. Restart `simulator-slow` before this test if you already used
the direct curl checks, because those requests consume the same health
counter.

The gRPC instance's HTTP discovery response advertises both diagnostics
transports. Its REST diagnostics endpoint is also available for directly
checking the JSON payload:

```sh
curl -i http://127.0.0.1:8083/health
curl -i http://127.0.0.1:8083/diagnostics
```

The following recipes change only the flaky instance. Edit
`demo/simulator-flaky.env`, keep all required values, then recreate the
container so configuration is re-read. PostgreSQL and the other simulators
remain running:

```sh
docker compose -f compose.yaml -f compose.simulators.yaml up -d --no-deps --force-recreate simulator-flaky
```

| Scenario | Set in `simulator-flaky.env` | Expected result |
| --- | --- | --- |
| Slow discovery | `HEALTH_BEHAVIOR=slow`, `FAILURE_COUNT=2`, `RESPONSE_DELAY=3s` | First two `/health` requests return 200 after about three seconds; the third returns immediately. `curl --max-time 1` cancels a delayed request, but it still consumes one of the two slow requests. |
| Slow REST diagnostics | `DIAGNOSTICS_BEHAVIOR=slow`, `FAILURE_COUNT=2`, `RESPONSE_DELAY=3s` | First two `/diagnostics` requests return 200 after about three seconds; the third returns immediately. |
| Counted REST diagnostics errors | `DIAGNOSTICS_BEHAVIOR=error`, `FAILURE_COUNT=2`, `ERROR_STATUS=503` | First two `/diagnostics` requests return 503; later requests return normal HTTP 200 diagnostics. |
| Incomplete diagnostics | `DIAGNOSTICS_BEHAVIOR=incomplete` | `/diagnostics` returns 200 with an empty firmware version and `invalid_diagnostics`. |
| Checksum failure | `DIAGNOSTICS_BEHAVIOR=healthy`, `CHECKSUM_ARGS=--fail` | `/diagnostics` returns 200 with null checksum and `checksum_unavailable`. |

For gRPC diagnostics behavior, change the gRPC instance's
`DIAGNOSTICS_BEHAVIOR`, `FAILURE_COUNT` or `RESPONSE_DELAY` and recreate
`simulator-grpc`; use a gRPC client against `127.0.0.1:9091` to observe it.
Its first `FAILURE_COUNT` `slow` requests are delayed; `flaky` and `error`
return `Unavailable` for that many requests. Subsequent requests succeed.

To restore the default flaky scenario, set
`HEALTH_BEHAVIOR=flaky`, `DIAGNOSTICS_BEHAVIOR=healthy`, `FAILURE_COUNT=2`,
`ERROR_STATUS=503`, and an empty `CHECKSUM_ARGS`, then recreate the container.
Changing an env file requires a restart/recreation, not an image rebuild.
Changes to simulator or checksum **code** require rebuilding, for example:

```sh
docker compose -f compose.yaml -f compose.simulators.yaml up -d --build --no-deps --force-recreate simulator-flaky
```

These are simulator endpoint checks only. They do not prove that the monitor
discovered, polled or persisted a device; use the main project README for the
monitor workflow.
