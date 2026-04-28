# system-routes

`system-routes` registers Fiber handlers for `/api/v1/system`.

It provides:

- system information (`command=info`)
- process resource usage (`command=resources`)
- runtime logger level read/update operations

## Register Routes

```go
app := fiber.New()

cfg := system.Config{
	UiEndPoint:               "https://openwifi.wlan.local",
	ServerCertificatePath:    "/etc/my-service/certs/restapi.pem",
	WebsocketCertificatePath: "/etc/my-service/certs/websocket.pem",
}

system.RegisterRoutes(cfg, app)
```

Registered endpoints:

- `GET /api/v1/system`
- `POST /api/v1/system`
- `OPTIONS /api/v1/system`

`OPTIONS` returns `204 No Content` and header `Allow: GET,POST,OPTIONS`.

## Config

```go
type Config struct {
	UiEndPoint               string `env:"SYSTEM_URI_UI" envDefault:"https://openwifi.wlan.local"`
	ServerCertificatePath    string `env:"INTERNAL_RESTAPI_HOST_CERT"`
	WebsocketCertificatePath string `env:"WEBSOCKET_HOST_CERT"`
}
```

Environment variables:

- `SYSTEM_URI_UI`
- `INTERNAL_RESTAPI_HOST_CERT`
- `WEBSOCKET_HOST_CERT`

## GET `/api/v1/system`

Query param `command` supports:

- `info`
- `resources`

Any other value returns `400` with:

```json
{
  "ErrorCode": 400,
  "ErrorDetails": "GET",
  "ErrorDescription": "1031: Invalid command."
}
```

### `command=info` response

Returns:

- `version`
- `uptime`
- `start`
- `os`
- `processors`
- `hostname`
- `UI`
- `certificates`

Version format:

- from `buildinfo.GetVersion()` and `buildinfo.GetCommitHash()`
- if commit hash is empty, `"unknown"` is used
- if version is empty, version field is only the commit hash
- otherwise version field is `<version>-<commitHash>`

Certificate list behavior:

- inputs: `ServerCertificatePath` and `WebsocketCertificatePath`
- empty path values are skipped
- valid cert entry:
  - `filename`: base file name
  - `expiresOn`: `cert.NotAfter.Unix()`
- missing file entry:
  - `filename`: `"file not found : <baseName>"`
  - `expiresOn`: `0`
- invalid PEM/cert entry:
  - `filename`: `"invalid cert file : <baseName>"`
  - `expiresOn`: `0`

### `command=resources` response

Returns:

- `numberOfFileDescriptors`
- `currRealMem`
- `peakRealMem`
- `currVirtMem`
- `peakVirtMem`

## POST `/api/v1/system`

Body supports `command`:

- `setloglevel`
- `getloglevels`
- `getloglevelnames`
- `getsubsystemnames`

`reload` is rejected with the same invalid-command `400` response as unsupported commands.

### `setloglevel`

Request:

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

Behavior:

- empty or missing `subsystems` returns invalid-parameters `400`
- `tag=all` applies the same level to all known subsystems with log level in lowercase
- unknown tags are still passed to `logger.UpdateSubsystemLevels`
- if update succeeds, returns:

```json
{
  "result": "success"
}
```

### `getloglevels`

Response shape:

```json
{
  "tagList": [
    {
      "tag": "subsystem-name",
      "value": "level"
    }
  ]
}
```

`tagList` is sorted by `tag` ascending.
`value` will be log level in lowercase

### `getloglevelnames`

Response shape:

```json
{
  "list": ["debug", "info", "warn", "error"]
}
```

Values come from `logger.GetAllLevels()`.

### `getsubsystemnames`

Response shape:

```json
{
  "list": ["auth", "system", "wifi"]
}
```

List is sorted ascending.

## Common POST Error Responses

Invalid command:

```json
{
  "ErrorCode": 400,
  "ErrorDetails": "POST",
  "ErrorDescription": "1031: Invalid command."
}
```

Invalid or missing parameters:

```json
{
  "ErrorCode": 400,
  "ErrorDetails": "POST",
  "ErrorDescription": "1018: Invalid or missing parameters."
}
```
