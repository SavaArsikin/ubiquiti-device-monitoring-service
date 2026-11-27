# REST simulator scenarios

The simulator serves `GET /health` for discovery and `GET /diagnostics` for
fake device details. The two endpoints have independent behaviors and request
counters. These are simulator responses, not evidence that the monitor polls
devices or supports real hardware.

From the repository root, start the prepared healthy and flaky instances with
PostgreSQL:

```sh
podman compose -f compose.yaml -f compose.simulators.yaml up --build
```

In another terminal, use `http://127.0.0.1:8081` for the healthy instance and
`http://127.0.0.1:8082` for the flaky instance. For Docker, replace
`podman compose` with `docker compose`. The full setup is in the
[project README](../README.md).

## Configuration

Compose mounts `simulator-healthy.env` and `simulator-flaky.env` into the
respective containers. The simulator reads the file named by `-config` on
startup; it does not use ambient environment variables. Every key below is
required; provide valid values, with an empty `CHECKSUM_ARGS` for no arguments.
Durations such as `2s` must be positive.

| Variable | Effect |
| --- | --- |
| `HTTP_ADDR` | Host and port on which the simulator listens. The demo uses `0.0.0.0:8081` inside each container. Compose maps host `127.0.0.1:8081` to healthy and `127.0.0.1:8082` to flaky, both to container port 8081. Changing the container port also requires changing its Compose mapping. |
| `READ_HEADER_TIMEOUT` | Maximum time allowed to read HTTP request headers; a client sending headers too slowly can be disconnected. |
| `SHUTDOWN_TIMEOUT` | Time allowed for graceful HTTP shutdown after a stop signal, before the server is forcibly closed. Compose's stop grace period is 15 seconds. |
| `HARDWARE_VERSION` | Fake hardware version returned as `hardware_version` in diagnostics. |
| `SOFTWARE_VERSION` | Fake software version returned as `software_version`. |
| `FIRMWARE_VERSION` | Fake firmware version returned as `firmware_version`; the incomplete scenario replaces it with an empty string. |
| `DEVICE_STATUS` | Fake device-reported `status` in diagnostics; it does not change endpoint reachability. |
| `HEALTH_BEHAVIOR` | Behavior of `/health`: `healthy`, `slow`, `flaky`, or `error`. Successful discovery returns HTTP 200 and `{"rest_diagnostics_path":"/diagnostics"}`. |
| `DIAGNOSTICS_BEHAVIOR` | Behavior of `/diagnostics`: `healthy`, `slow`, `flaky`, `error`, or `incomplete`. Successful diagnostics returns HTTP 200 with versions, status and checksum. |
| `RESPONSE_DELAY` | Delay for an endpoint in `slow` mode only. A client timeout shorter than this delay can cancel the request. |
| `FAILURE_COUNT` | Number of initial requests that fail **per endpoint** in `flaky` mode; must be zero or greater. Later requests succeed. Restarting the instance resets both counters. |
| `ERROR_STATUS` | HTTP status used for `error` and initial `flaky` responses; must be 400–599. |
| `CHECKSUM_BINARY` | Executable invoked for successful diagnostics. Compose uses `/app/checksumstub`; an unavailable executable prevents simulator startup. |
| `CHECKSUM_ARGS` | Required key containing comma-separated process arguments. Leave it empty for normal behavior; `--fail` makes the supplied stub fail. Arguments must be supported by the selected executable. |
| `CHECKSUM_TIMEOUT` | Maximum time for each checksum process. Timeout, invalid output or process failure produces an unavailable checksum rather than an unreachable device. |

`healthy` responds normally; `slow` waits `RESPONSE_DELAY` before responding;
`error` always returns `ERROR_STATUS`; `flaky` returns that status for the first
`FAILURE_COUNT` requests to that endpoint, then responds normally. With
`incomplete` diagnostics, the response is still HTTP 200 but has an empty
`firmware_version`, `checksum: null` and `failure_category:
invalid_diagnostics`.

For normal diagnostics the simulator starts a one-shot checksum process,
passes JSON versions and status (without checksum) on stdin, and expects
`{"checksum":"..."}` on stdout. The bundled stub returns a SHA-256 hash of
canonicalized fake input; it is only a wiring example, not a real device
checksum. The stub limits input to 64 KiB, and the simulator limits process
stdout to 64 KiB and bounds execution by request context and
`CHECKSUM_TIMEOUT`. On failure diagnostics remains HTTP 200 with
`checksum: null` and `failure_category: checksum_unavailable`.

## Try scenarios

Start with the included files. Healthy discovery and diagnostics each return
HTTP 200:

```sh
curl -i http://127.0.0.1:8081/health
curl -i http://127.0.0.1:8081/diagnostics
```

The included flaky instance has `HEALTH_BEHAVIOR=flaky`,
`FAILURE_COUNT=2`, and `ERROR_STATUS=503`. Reset its counters, then observe
503, 503, and 200:

```sh
podman compose -f compose.yaml -f compose.simulators.yaml restart simulator-flaky
curl -i http://127.0.0.1:8082/health
curl -i http://127.0.0.1:8082/health
curl -i http://127.0.0.1:8082/health
```

To try the following scenarios, edit `demo/simulator-flaky.env`, setting the
listed keys while keeping the other required keys present. After each edit,
recreate only the flaky instance so even file-replacing editors refresh the
bind mount; PostgreSQL and the healthy simulator keep running:

```sh
podman compose -f compose.yaml -f compose.simulators.yaml up -d --no-deps --force-recreate simulator-flaky
```

| Scenario | Set in `simulator-flaky.env` | Request and expected result |
| --- | --- | --- |
| Slow discovery | `HEALTH_BEHAVIOR=slow`, `RESPONSE_DELAY=3s` | `curl -i http://127.0.0.1:8082/health` returns 200 after about three seconds; `curl --max-time 1` times out. |
| Slow diagnostics | `DIAGNOSTICS_BEHAVIOR=slow`, `RESPONSE_DELAY=3s` | `curl -i http://127.0.0.1:8082/diagnostics` returns 200 after about three seconds. |
| Permanent diagnostics error | `DIAGNOSTICS_BEHAVIOR=error`, `ERROR_STATUS=503` | `curl -i http://127.0.0.1:8082/diagnostics` returns 503 on every request. Choose another 400–599 status to change the response. |
| Incomplete diagnostics | `DIAGNOSTICS_BEHAVIOR=incomplete` | `curl -i http://127.0.0.1:8082/diagnostics` returns 200 with empty firmware and `invalid_diagnostics`. |
| Checksum failure | `DIAGNOSTICS_BEHAVIOR=healthy`, `CHECKSUM_ARGS=--fail` | `curl -i http://127.0.0.1:8082/diagnostics` returns 200 with null checksum and `checksum_unavailable`. |

Scenarios are independent: reset settings from a previous experiment as
needed. To restore the original flaky demo, use `HEALTH_BEHAVIOR=flaky`,
`DIAGNOSTICS_BEHAVIOR=healthy`, `FAILURE_COUNT=2`, `ERROR_STATUS=503`, and an
empty `CHECKSUM_ARGS`, then recreate the instance.

Env-file changes need recreation or restart, not an image rebuild. Simulator or
checksum **code** changes require rebuilding and recreating the affected
instance, for example:

```sh
podman compose -f compose.yaml -f compose.simulators.yaml up -d --build --no-deps --force-recreate simulator-flaky
```
