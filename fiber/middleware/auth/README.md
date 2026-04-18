# Auth Middleware (`fiber/middleware/auth`)

This package provides reusable Fiber middlewares for:

- Internal/private API key authentication
- Public auth with either bearer token or API key authentication

## Usage

Import this package in code:

```go
import "github.com/routerarchitects/ow-common-mods/fiber/middleware/auth"
```

## Internal Auth (`RequireInternalAPIKey`)

Use this middleware for service-to-service/private endpoints.

### Validation Flow

1. Reads internal caller header (`X-INTERNAL-NAME` provides caller metadata)
2. Reads API key header (`X-API-KEY` by default for authentication)
3. Compares with configured `ExpectedAPIKey` (constant-time comparison)

If any step fails, request is rejected with `401` (or custom `OnUnauthorized` handler).

### Config

- `InternalNameHeader` (default: `X-INTERNAL-NAME` for caller metadata)
- `APIKeyHeader` (default: `X-API-KEY`)
- `ExpectedAPIKey` (**required**)
- `OnUnauthorized` (optional custom unauthorized writer)

### Example

```go
internalAuth, err := auth.RequireInternalAPIKey(auth.InternalAPIKeyConfig{
    ExpectedAPIKey: os.Getenv("TOPOLOGY_PRIVATE_API_KEY"),
})
if err != nil {
    return err
}

app.Use(internalAuth)
```

## Public Auth (`RequirePublicAuth`)

Use this middleware for public endpoints with dynamic credential handling:

1. If `X-API-KEY` is present, API-key validation is attempted first.
2. If API-key validation succeeds, request is allowed immediately and bearer validation is skipped.
3. If API-key validation fails and `Authorization: Bearer <token>` is present, bearer validation is attempted.

### Validation Flow

1. Reads API key header (`X-API-KEY` by default)
2. Reads authorization header (`Authorization` by default)
3. Validates credentials against:
   - `Validator.ValidateAPIKey(ctx, apiKey)`
   - `Validator.ValidateToken(ctx, token)`

If validation fails, request is rejected with `401` (or custom `OnValidationError`/`OnUnauthorized`).

### Config

- `AuthorizationHeader` (default: `Authorization`)
- `BearerPrefix` (default: `Bearer `)
- `APIKeyHeader` (default: `X-API-KEY`)
- `ValidationTimeout` (default: `5s` per validator call)
- `Validator` (**required**)
- `OnUnauthorized` (optional custom unauthorized writer)
- `OnValidationError` (optional custom validator error mapper)

### Example

```go
type authServiceValidator struct {
    client AuthRPCClient
}

func (v authServiceValidator) ValidateToken(ctx context.Context, token string) error {
    return v.client.ValidateToken(ctx, token)
}

func (v authServiceValidator) ValidateAPIKey(ctx context.Context, apiKey string) error {
    return v.client.ValidateAPIKey(ctx, apiKey)
}

publicAuth, err := auth.RequirePublicAuth(auth.PublicAuthConfig{
    ValidationTimeout: 3 * time.Second,
    Validator: authServiceValidator{client: authClient},
})
if err != nil {
    return err
}

app.Use(publicAuth)
```

## Important Notes

- `RequireInternalAPIKey` returns an error if `ExpectedAPIKey` is empty.
- `RequirePublicAuth` returns an error if `Validator` is nil.
- Validator methods receive a timeout-bound context derived from `fiber.Ctx.Context()`.
- Raw `Authorization: <token>` is rejected; bearer auth requires `Authorization: Bearer <token>`.
