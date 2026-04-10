package requestlog

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
)

func TestRequestLogger_LogsSuccessRequest(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&out, nil))

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
	if !strings.Contains(logLine, `"level":"INFO"`) {
		t.Fatalf("log does not contain INFO level: %s", logLine)
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

func TestRequestLogger_LogsErrorRequest(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&out, nil))

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
	if !strings.Contains(logLine, `"request_id":"`) {
		t.Fatalf("log does not contain request_id: %s", logLine)
	}
}

func TestRequestLogger_PropagatesIncomingRequestID(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&out, nil))

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

func TestRequestLogger_DoesNotLogQueryString(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&out, nil))

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
