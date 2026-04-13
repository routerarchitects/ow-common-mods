package apperrors

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
)

// Code defines common application error codes.
type Code string

const (
	CodeNotFound     Code = "NOT_FOUND"
	CodeInvalidInput Code = "INVALID_INPUT"
	CodeUnauthorized Code = "UNAUTHORIZED"
	CodeForbidden    Code = "FORBIDDEN"
	CodeConflict     Code = "CONFLICT"
	CodeInternal     Code = "INTERNAL_SERVER"
	CodeUnknown      Code = "UNKNOWN"
)

// Frame captures where the error was created.
type Frame struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Function string `json:"function"`
}

// Error defines the semantic application error.
type Error struct {
	Code    Code           `json:"code"`
	Message string         `json:"message"`
	Cause   error          `json:"-"`
	Meta    map[string]any `json:"meta,omitempty"`
	Frame   Frame          `json:"frame"`
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap allows errors.Is / errors.As to work.
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// New creates a new application error.
func New(code Code, message string) *Error {
	return &Error{
		Code:    code,
		Message: message,
		Frame:   captureFrame(1),
	}
}

// Wrap creates a new application error with a cause.
func Wrap(code Code, message string, cause error) *Error {
	return &Error{
		Code:    code,
		Message: message,
		Cause:   cause,
		Frame:   captureFrame(1),
	}
}

// WithMeta attaches metadata to the error.
func (e *Error) WithMeta(meta map[string]any) *Error {
	if e == nil {
		return nil
	}
	e.Meta = meta
	return e
}

// WithCause attaches a cause to the error.
func (e *Error) WithCause(cause error) *Error {
	if e == nil {
		return nil
	}
	e.Cause = cause
	return e
}

func captureFrame(skip int) Frame {
	pc, file, line, ok := runtime.Caller(skip + 1)
	if !ok {
		return Frame{}
	}

	fn := runtime.FuncForPC(pc)
	funcName := ""
	if fn != nil {
		funcName = fn.Name()
	}

	return Frame{
		File:     filepath.Base(file),
		Line:     line,
		Function: funcName,
	}
}

// CodeOf returns the application code if present.
func CodeOf(err error) Code {
	var appErr *Error
	if errors.As(err, &appErr) && appErr != nil {
		return appErr.Code
	}
	return CodeUnknown
}

// MessageOf returns the application message if present.
func MessageOf(err error) string {
	var appErr *Error
	if errors.As(err, &appErr) && appErr != nil && appErr.Message != "" {
		return appErr.Message
	}
	if err != nil {
		return err.Error()
	}
	return ""
}

// MetaOf returns metadata if present.
func MetaOf(err error) map[string]any {
	var appErr *Error
	if errors.As(err, &appErr) && appErr != nil && appErr.Meta != nil {
		return appErr.Meta
	}
	return nil
}

// FrameOf returns the creation frame if present.
func FrameOf(err error) Frame {
	var appErr *Error
	if errors.As(err, &appErr) && appErr != nil {
		return appErr.Frame
	}
	return Frame{}
}

// GetLog returns a recursive log string with all wrapped error info.
func GetLog(err error) string {
	if err == nil {
		return ""
	}

	var b strings.Builder
	writeLog(&b, err, 0)
	return b.String()
}

func writeLog(b *strings.Builder, err error, depth int) {
	if err == nil {
		return
	}

	indent := strings.Repeat("  ", depth)

	var appErr *Error
	if errors.As(err, &appErr) && appErr != nil {
		fmt.Fprintf(
			b,
			"%scode=%s message=%q file=%s line=%d function=%s",
			indent,
			appErr.Code,
			appErr.Message,
			appErr.Frame.File,
			appErr.Frame.Line,
			appErr.Frame.Function,
		)

		if len(appErr.Meta) > 0 {
			fmt.Fprintf(b, " meta=%v", appErr.Meta)
		}
		b.WriteString("\n")

		if appErr.Cause != nil {
			writeLog(b, appErr.Cause, depth+1)
		}
		return
	}

	fmt.Fprintf(b, "%serror=%q\n", indent, err.Error())

	unwrapped := errors.Unwrap(err)
	if unwrapped != nil {
		writeLog(b, unwrapped, depth+1)
	}
}
