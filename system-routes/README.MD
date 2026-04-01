# subsystem-modules

`subsystem-modules` provides Fiber routes for `/api/v1/system` so a service can expose runtime system details and manage subsystem log levels through the shared `ra-common-mods/logger` package.

## What It Does

The module registers these handlers:

- `GET /api/v1/system?command=info`
- `GET /api/v1/system?command=resources`
- `POST /api/v1/system`
- `OPTIONS /api/v1/system`

At runtime, it works like this:

1. A service creates `subsystemmodules.Config`.
2. The service constructs the handler with `NewSubsytems(config)`.
3. The handler is attached to a Fiber router with `RegisterFiberRoutes(...)`.
4. Incoming requests are routed by `command`.
5. Log-level commands read or update subsystem state through `logger.GetSubsystemLevels()` and `logger.UpdateSubsystemLevels(...)`.
6. System info responses include service version, uptime, host details, configured UI endpoint, and certificate expiry details.

## API Behavior

### GET `/api/v1/system`

Supported query values for `command`:

- `info`: returns version, uptime, start time, OS, CPU count, hostname, UI endpoint, and certificate expiry metadata.
- `resources`: returns process memory usage and number of open file descriptors.

Any other `command` returns a `400` response.

### POST `/api/v1/system`

Supported JSON `command` values:

- `setloglevel`: updates one or more subsystem log levels.
- `getloglevels`: returns the current subsystem log-level mapping as `tagList`.
- `getloglevelnames`: returns the supported log level names from the logger package.
- `getsubsystemnames`: returns the available subsystem names.

Any other `command` returns a `400` response.

Example request for `setloglevel`:

```json
{
  "command": "setloglevel",
  "subsystems": [
    {
      "tag": "all",
      "value": "debug"
    }
  ]
}
```

Notes:

- If `tag` is `all`, the same log level is applied to every known subsystem.
- `getloglevels` returns the level values in lowercase.
- `getsubsystemnames` returns subsystem names in lowercase.

## Environment Variables

The module reads configuration from these environment variables:

- `UI_ENDPOINT`: UI URL returned in the `info` response.
- `CERTS`: comma-separated list of certificate file paths.

## Configuration

The config type is:

```go
type Config struct {
	UI_EndPoint string   `env:"UI_ENDPOINT" envDefault:"https://openwifi.wlan.local"`
	CERTS       []string `env:"CERTS" envSeparator:","`
}
```

Example environment configuration:

```bash
export UI_ENDPOINT="https://openwifi.wlan.local"
export CERTS="/etc/my-service/certs/restapi-ca.pem,/etc/my-service/certs/server.pem,/etc/my-service/certs/restapi-ca.pem,/etc/my-service/certs/server.pem"
```

## Certificate Handling

The `info` response reads certificate metadata from the paths configured in `CERTS`.

Current behavior:

- Each `CERTS` entry must be a direct path to a PEM certificate file.
- Directory paths are not supported by the current implementation.
- For each file, the response includes:
  - `filename`
  - `expiresOn` as a Unix timestamp

If any configured certificate path cannot be read or parsed, the `info` request returns an internal error for the `certificates` subsystem.

## Version Handling

The module reads service version information from `github.com/routerarchitects/ra-common-mods/build-info` and includes it in the `info` response.

## Dependencies

- `github.com/gofiber/fiber/v3`
- `github.com/routerarchitects/ra-common-mods/logger`
- `github.com/routerarchitects/ra-common-mods/build-info`
