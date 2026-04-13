# system-routes

`system-routes` provides Fiber handlers for `/api/v1/system`. It exposes runtime system details and log-level management backed by `ra-common-mods/logger`.

## What Changed

The current module behavior is:

- Route handling now lives in [system.go].
- The config now uses explicit certificate path fields.
- Build version data is read from `github.com/routerarchitects/ra-common-mods/buildinfo`.

## Registered Routes

- `GET /api/v1/system?command=info`
- `GET /api/v1/system?command=resources`
- `POST /api/v1/system`
- `OPTIONS /api/v1/system`

Typical integration flow:

1. Build a `system.Config`.
2. Register routes with `RegisterRoutes(config, router)`.

Example:

```go
app := fiber.New()
cfg := system.Config{}

system.RegisterRoutes(cfg, app)
```

## API Behavior

### GET `/api/v1/system`

Supported `command` query values:

- `info`: returns version, uptime, start time, OS, CPU count, hostname, configured UI endpoint, and certificate expiry metadata.
- `resources`: returns process memory usage and number of open file descriptors.

Any other value returns `400 Bad Request`.

### POST `/api/v1/system`

Supported JSON `command` values:

- `setloglevel`: updates one or more subsystem log levels.
- `getloglevels`: returns the current subsystem log-level mapping as `tagList`.
- `getloglevelnames`: returns the supported log level names from the logger package.
- `getsubsystemnames`: returns the available subsystem names.

`reload` is currently rejected with the same `400 Bad Request` response as any unsupported command.

Example `setloglevel` request:

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
- Unknown subsystem tags are still forwarded to `logger.UpdateSubsystemLevels(...)`.
- `getloglevels` returns level values in lowercase.
- `getsubsystemnames` returns subsystem names sorted in ascending order.

## Configuration

Current config type:

```go
type Config struct {
	UiEndPoint               string `env:"SYSTEM_URI_UI" envDefault:"https://openwifi.wlan.local"`
	ServerCertificatePath    string `env:"INTERNAL_RESTAPI_HOST_CERT"`
	WebsocketCertificatePath string `env:"WEBSOCKET_HOST_CERT"`
}
```

Environment variables:

- `SYSTEM_URI_UI`: UI URL returned in the `info` response.
- `INTERNAL_RESTAPI_HOST_CERT`: PEM certificate path used for the REST API certificate entry.
- `WEBSOCKET_HOST_CERT`: PEM certificate path used for the WebSocket certificate entry.

Example:

```bash
export SYSTEM_URI_UI="https://openwifi.wlan.local"
export INTERNAL_RESTAPI_HOST_CERT="/etc/my-service/certs/restapi.pem"
export WEBSOCKET_HOST_CERT="/etc/my-service/certs/websocket.pem"
```

## `info` Response Details

The `info` response includes:

- `version`
- `uptime`
- `start`
- `os`
- `processors`
- `hostname`
- `UI`
- `certificates`

Version handling:

- The module reads version information from `buildinfo.GetVersion()` and `buildinfo.GetCommitHash()`.
- If the version is empty, the commit hash is returned by itself.
- Otherwise the response format is `<version>-<commitHash>`.

Certificate handling:

- The module reads certificate data from `INTERNAL_RESTAPI_HOST_CERT` and `WEBSOCKET_HOST_CERT`.
- Empty paths are skipped.
- Valid PEM certificates return:
  - `filename`
  - `expiresOn` as a Unix timestamp
- Missing files return `filename: "file not found"` with `expiresOn: 0`.
- Invalid PEM or invalid certificate data return `filename: "invalid cert file"` with `expiresOn: 0`.

## Dependencies

- `github.com/gofiber/fiber/v3`
- `github.com/routerarchitects/ra-common-mods/buildinfo`
- `github.com/routerarchitects/ra-common-mods/logger`
