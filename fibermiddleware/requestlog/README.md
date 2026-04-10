# Request Log Middleware (`fibermiddleware/requestlog`)

This package provides a Fiber middleware for structured request logging using `slog`.

## Installation

```bash
go get github.com/routerarchitects/ow-common-mods/fibermiddleware
```

## Middleware

```go
func RequestLogger(logger *slog.Logger) fiber.Handler
```

- Uses provided logger
- Falls back to `slog.Default()` when logger is `nil`

## Behavior

For each request, middleware logs:

1. `request started`
2. `request completed`

It also ensures request-id correlation:

- Reads incoming `X-Request-ID` when present
- Generates one if missing
- Sets `X-Request-ID` in response header
- Includes `request_id` in both start and completion logs

## Logged Fields

### `request started`

- `request_id`
- `method`
- `path`
- `client_ip`

### `request completed`

- `request_id`
- `status`
- `latency_ms`
- `error` (only when downstream returns an error)

## Example

```go
logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

app := fiber.New()
app.Use(requestlog.RequestLogger(logger))
```

## Notes

- This middleware does not log request body.
- This middleware does not log query parameters.
- For application logs in deeper layers, pass request context downstream and reuse `X-Request-ID` from headers/context strategy used by your service.
