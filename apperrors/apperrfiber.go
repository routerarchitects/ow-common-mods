package apperrors

import (
	"github.com/gofiber/fiber/v3"
)

type HTTPErrorInfo struct {
	Status      int
	Description string
}

var errorInfoMap = map[Code]HTTPErrorInfo{
	CodeInvalidInput: {Status: fiber.StatusBadRequest, Description: "Bad request."},
	CodeUnauthorized: {Status: fiber.StatusUnauthorized, Description: "Unauthorized."},
	CodeForbidden:    {Status: fiber.StatusForbidden, Description: "Forbidden."},
	CodeNotFound:     {Status: fiber.StatusNotFound, Description: "Resource does not exist."},
	CodeConflict:     {Status: fiber.StatusConflict, Description: "Conflict."},
	CodeInternal:     {Status: fiber.StatusInternalServerError, Description: "Internal Server Error."},
	CodeUnknown:      {Status: fiber.StatusInternalServerError, Description: "Internal Server Error."},
}

var defaultHTTPErrorInfo = HTTPErrorInfo{
	Status:      fiber.StatusInternalServerError,
	Description: "Internal Server Error.",
}

func InfoOf(err error) HTTPErrorInfo {
	code := CodeOf(err)
	if info, ok := errorInfoMap[code]; ok {
		return info
	}
	return defaultHTTPErrorInfo
}

// ResponseBody defines the standard Fiber JSON error response.
type ResponseBody struct {
	Code    Code           `json:"code"`
	Message string         `json:"message"`
	Meta    map[string]any `json:"meta,omitempty"`
}

func Respond(c fiber.Ctx, err error) error {
	info := InfoOf(err)

	body := ResponseBody{
		Code:    CodeOf(err),
		Message: MessageOf(err),
		Meta:    MetaOf(err),
	}

	return c.Status(info.Status).JSON(body)
}
