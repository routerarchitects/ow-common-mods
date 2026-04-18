package requestlog

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
)

const requestIDHeader = "X-Request-ID"

// RequestLogger logs one line per request using the provided slog logger.
func RequestLogger(logger *slog.Logger) fiber.Handler {
	if logger == nil {
		logger = slog.Default()
	}

	return func(c fiber.Ctx) error {
		requestID := resolveRequestID(c.Get(requestIDHeader))
		c.Set(requestIDHeader, requestID)

		// TODO Need to use logger module from ra-common-mods to
		// propagate the request_id to the downstream consumers.

		start := time.Now()
		logger.Debug(
			"request started",
			"request_id", requestID,
			"method", c.Method(),
			"path", c.Path(),
			"client_ip", c.IP(),
		)

		err := c.Next()
		duration := time.Since(start)
		status := c.Response().StatusCode()
		if err != nil {
			status = statusFromError(err, status)
		}

		args := []any{
			"request_id", requestID,
			"status", status,
			"latency_ms", duration.Milliseconds(),
		}

		if err != nil {
			args = append(args, "error", err.Error())
			logger.Error("request completed", args...)
			return err
		}

		logger.Debug("request completed", args...)
		return nil
	}
}

func statusFromError(err error, currentStatus int) int {
	// Keep status explicitly set by downstream handlers.
	if currentStatus >= 400 {
		return currentStatus
	}

	var fiberErr *fiber.Error
	if errors.As(err, &fiberErr) {
		return fiberErr.Code
	}

	return fiber.StatusInternalServerError
}

func resolveRequestID(headerValue string) string {
	requestID := strings.TrimSpace(headerValue)
	if requestID != "" {
		return requestID
	}

	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
