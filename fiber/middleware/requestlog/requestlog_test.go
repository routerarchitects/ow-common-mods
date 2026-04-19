package requestlog

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
)

// Verifies that a successful request logs expected debug fields and sets response request-id.
func TestRequestLogger_LogsSuccessRequest(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug}))

	app := fiber.New()
	app.Use(RequestLogger(logger))
	app.Get("/health", func(c fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	logLine := out.String()
	if !strings.Contains(logLine, `"level":"DEBUG"`) {
		t.Fatalf("log does not contain DEBUG level: %s", logLine)
	}
	if !strings.Contains(logLine, `"method":"GET"`) {
		t.Fatalf("log does not contain method: %s", logLine)
	}
	if !strings.Contains(logLine, `"path":"/health"`) {
		t.Fatalf("log does not contain path: %s", logLine)
	}
	if !strings.Contains(logLine, `"status":200`) {
		t.Fatalf("log does not contain status: %s", logLine)
	}
	if !strings.Contains(logLine, `"request_id":"`) {
		t.Fatalf("log does not contain request_id: %s", logLine)
	}
	if got := resp.Header.Get("X-Request-ID"); got == "" {
		t.Fatalf("response does not contain X-Request-ID header")
	}
}

// Verifies that a Fiber error is logged at error level with the mapped status and request-id.
func TestRequestLogger_LogsErrorRequest(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug}))

	app := fiber.New()
	app.Use(RequestLogger(logger))
	app.Get("/boom", func(c fiber.Ctx) error {
		return fiber.ErrUnauthorized
	})

	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}

	logLine := out.String()
	if !strings.Contains(logLine, `"level":"ERROR"`) {
		t.Fatalf("log does not contain ERROR level: %s", logLine)
	}
	if !strings.Contains(logLine, `"error":"Unauthorized"`) {
		t.Fatalf("log does not contain error: %s", logLine)
	}
	if !strings.Contains(logLine, `"status":401`) {
		t.Fatalf("log does not contain status 401: %s", logLine)
	}
	if !strings.Contains(logLine, `"request_id":"`) {
		t.Fatalf("log does not contain request_id: %s", logLine)
	}
}

// Verifies that a generic downstream error maps to internal-server-error status in logs.
func TestRequestLogger_LogsStatusOnGenericError(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug}))

	app := fiber.New()
	app.Use(RequestLogger(logger))
	app.Get("/generic-error", func(c fiber.Ctx) error {
		return errors.New("boom")
	})

	req := httptest.NewRequest(http.MethodGet, "/generic-error", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusInternalServerError)
	}

	logLine := out.String()
	if !strings.Contains(logLine, `"level":"ERROR"`) {
		t.Fatalf("log does not contain ERROR level: %s", logLine)
	}
	if !strings.Contains(logLine, `"status":500`) {
		t.Fatalf("log does not contain status 500: %s", logLine)
	}
}

// Verifies that an incoming request-id is echoed to response header and reused in both log lines.
func TestRequestLogger_PropagatesIncomingRequestID(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug}))

	app := fiber.New()
	app.Use(RequestLogger(logger))
	app.Get("/ping", func(c fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	const rid = "req-12345"
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("X-Request-ID", rid)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	defer resp.Body.Close()

	if got := resp.Header.Get("X-Request-ID"); got != rid {
		t.Fatalf("response X-Request-ID = %q, want %q", got, rid)
	}

	logLine := out.String()
	if strings.Count(logLine, `"request_id":"req-12345"`) != 2 {
		t.Fatalf("expected request_id in both start and complete logs, got logs: %s", logLine)
	}
}

// Verifies that query-string values are not included in request logs.
func TestRequestLogger_DoesNotLogQueryString(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug}))

	app := fiber.New()
	app.Use(RequestLogger(logger))
	app.Get("/search", func(c fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/search?name=nitin&role=admin", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	defer resp.Body.Close()

	logLine := out.String()
	if strings.Contains(logLine, `"query":`) {
		t.Fatalf("log should not contain query string: %s", logLine)
	}
}

// Verifies that nil logger input falls back to slog.Default and still handles the request.
func TestRequestLogger_UsesDefaultLoggerWhenNil(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	prevDefault := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() {
		slog.SetDefault(prevDefault)
	})

	app := fiber.New()
	app.Use(RequestLogger(nil))
	app.Get("/default-logger", func(c fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/default-logger", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	defer resp.Body.Close()

	if got := resp.Header.Get("X-Request-ID"); got == "" {
		t.Fatalf("response does not contain X-Request-ID header")
	}
	logLine := out.String()
	if !strings.Contains(logLine, `"path":"/default-logger"`) {
		t.Fatalf("log does not contain request path from default logger: %s", logLine)
	}
}

// Verifies that generated request-id is reused in start/end logs and response header consistently.
func TestRequestLogger_GeneratedRequestIDIsConsistentAcrossLogsAndResponse(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug}))

	app := fiber.New()
	app.Use(RequestLogger(logger))
	app.Get("/generated-id", func(c fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/generated-id", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	defer resp.Body.Close()

	responseRID := resp.Header.Get("X-Request-ID")
	if responseRID == "" {
		t.Fatalf("response does not contain X-Request-ID header")
	}

	re := regexp.MustCompile(`"request_id":"([^"]+)"`)
	matches := re.FindAllStringSubmatch(out.String(), -1)
	if len(matches) != 2 {
		t.Fatalf("expected request_id in both start and end logs, got %d matches in logs: %s", len(matches), out.String())
	}
	if matches[0][1] != matches[1][1] {
		t.Fatalf("request_id differs between start and end logs: %q vs %q", matches[0][1], matches[1][1])
	}
	if matches[0][1] != responseRID {
		t.Fatalf("request_id in logs (%q) does not match response header (%q)", matches[0][1], responseRID)
	}
}

// Verifies that statusFromError keeps a downstream pre-set >=400 status for generic errors.
func TestStatusFromError_PreservesCurrentErrorStatus(t *testing.T) {
	t.Parallel()

	got := statusFromError(errors.New("boom"), fiber.StatusTeapot)
	if got != fiber.StatusTeapot {
		t.Fatalf("statusFromError() = %d, want %d", got, fiber.StatusTeapot)
	}
}

// Verifies that incoming request-id header values are trimmed before reuse in logs and response.
func TestRequestLogger_TrimWhitespaceAroundIncomingRequestID(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug}))

	app := fiber.New()
	app.Use(RequestLogger(logger))
	app.Get("/trim-rid", func(c fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/trim-rid", nil)
	req.Header.Set("X-Request-ID", "   req-trim-123   ")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	defer resp.Body.Close()

	if got := resp.Header.Get("X-Request-ID"); got != "req-trim-123" {
		t.Fatalf("response X-Request-ID = %q, want %q", got, "req-trim-123")
	}
	if strings.Count(out.String(), `"request_id":"req-trim-123"`) != 2 {
		t.Fatalf("expected trimmed request_id in both start and complete logs, got logs: %s", out.String())
	}
}

// Verifies current contract: generated request-id is not propagated to downstream request headers yet.
func TestRequestLogger_DoesNotPropagateGeneratedRequestIDDownstreamYet(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug}))

	var downstreamSeenRID string
	app := fiber.New()
	app.Use(RequestLogger(logger))
	app.Get("/downstream-rid", func(c fiber.Ctx) error {
		downstreamSeenRID = c.Get("X-Request-ID")
		return c.SendStatus(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/downstream-rid", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	defer resp.Body.Close()

	if downstreamSeenRID != "" {
		t.Fatalf("expected no downstream request-id propagation yet, got %q", downstreamSeenRID)
	}
	if got := resp.Header.Get("X-Request-ID"); got == "" {
		t.Fatalf("response does not contain generated X-Request-ID header")
	}
}
