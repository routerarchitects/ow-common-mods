package logger_routes

import (
	"os"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/routerarchitects/ra-common-mods/logger"
)

var processStartUnix = atomic.Int64{}

func init() {
	processStartUnix.Store(time.Now().Unix())
}

var supportedLogLevelNames = []string{
	"none",
	"fatal",
	"critical",
	"error",
	"warning",
	"notice",
	"information",
	"debug",
	"trace",
}

type setLogLevelSubsystem struct {
	Tag   string `json:"tag"`
	Value string `json:"value"`
}

type systemPostRequest struct {
	Command    string                 `json:"command"`
	Subsystems []setLogLevelSubsystem `json:"subsystems"`
	Extra      map[string]interface{} `json:"-"`
}

// RegisterFiberRoutes registers the system command routes.
func RegisterFiberRoutes(r fiber.Router) {
	r.Get("/api/v1/system", handleSystemGet)
	r.Post("/api/v1/system", handleSystemPost)
	r.Options("/api/v1/system", handleSystemOptions)
}

func handleSystemOptions(c fiber.Ctx) error {
	c.Set("Allow", "GET,POST,OPTIONS")
	return c.SendStatus(fiber.StatusNoContent)
}

func handleSystemGet(c fiber.Ctx) error {
	switch strings.TrimSpace(c.Query("command")) {
	case "info":
		return c.JSON(getSystemInfo())
	case "extraConfiguration":
		return c.JSON(fiber.Map{
			"additionalConfiguration": false,
		})
	case "resources":
		return c.JSON(getResourceUsage())
	default:
		return c.Status(fiber.StatusBadRequest).JSON(invalidCommandResponse("GET"))
	}
}

func handleSystemPost(c fiber.Ctx) error {
	var req systemPostRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(invalidParametersResponse())
	}

	switch strings.TrimSpace(req.Command) {
	case "setloglevel":
		return handleSetLogLevel(c, req.Subsystems)
	case "getloglevels":
		return c.JSON(fiber.Map{
			"tagList": getSubsystemTagList(),
		})
	case "getloglevelnames":
		return c.JSON(fiber.Map{
			"list": supportedLogLevelNames,
		})
	case "getsubsystemnames":
		return c.JSON(fiber.Map{
			"list": getSubsystemNames(),
		})
	case "reload":
		return c.Status(fiber.StatusBadRequest).JSON(invalidCommandResponse("POST"))
	default:
		return c.Status(fiber.StatusBadRequest).JSON(invalidCommandResponse("POST"))
	}
}

func handleSetLogLevel(c fiber.Ctx, rawSubsystems []setLogLevelSubsystem) error {
	if len(rawSubsystems) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(invalidParametersResponse())
	}

	current := logger.GetSubsystemLevels()
	updates := map[string]string{}

	for _, item := range rawSubsystems {

		tag := item.Tag
		value := item.Value

		normalized, ok := normalizeIncomingLevel(value)
		if !ok {
			return c.Status(fiber.StatusBadRequest).JSON(invalidParametersResponse())
		}

		if strings.EqualFold(tag, "all") {
			for name := range current {
				updates[name] = normalized
			}
			continue
		}

		// Keep success semantics even when the subsystem name is unknown.
		updates[tag] = normalized
	}

	if err := logger.UpdateSubsystemLevels(updates); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(invalidParametersResponse())
	}

	return c.JSON(successResponse())
}

func getSystemInfo() fiber.Map {
	hostname, _ := os.Hostname()
	version := "unknown"
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		version = info.Main.Version
	}

	return fiber.Map{
		"version":      version,
		"uptime":       time.Now().Unix() - processStartUnix.Load(),
		"start":        processStartUnix.Load(),
		"os":           runtime.GOOS,
		"processors":   runtime.NumCPU(),
		"hostname":     hostname,
		"ui":           "",
		"certificates": []fiber.Map{},
	}
}

func getResourceUsage() fiber.Map {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	return fiber.Map{
		"numberOfFileDescriptors": countOpenFileDescriptors(),
		"currRealMem":             mem.Alloc,
		"peakRealMem":             mem.HeapSys,
		"currVirtMem":             mem.Sys,
		"peakVirtMem":             mem.TotalAlloc,
	}
}

func countOpenFileDescriptors() int {
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		return 0
	}
	return len(entries)
}

func getSubsystemTagList() []fiber.Map {
	levels := logger.GetSubsystemLevels()
	names := make([]string, 0, len(levels))
	for name := range levels {
		names = append(names, name)
	}
	sort.Strings(names)

	tagList := make([]fiber.Map, 0, len(names))
	for _, name := range names {
		tagList = append(tagList, fiber.Map{
			"tag":   name,
			"value": normalizeOutgoingLevel(levels[name]),
		})
	}
	return tagList
}

func getSubsystemNames() []string {
	levels := logger.GetSubsystemLevels()
	names := make([]string, 0, len(levels))
	for name := range levels {
		names = append(names, strings.ToLower(name))
	}
	sort.Strings(names)
	return names
}

func normalizeIncomingLevel(level string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "none", "fatal", "critical", "error":
		return "error", true
	case "warning":
		return "warn", true
	case "notice", "information":
		return "info", true
	case "debug", "trace":
		return "debug", true
	case "warn", "info":
		return strings.ToLower(strings.TrimSpace(level)), true
	default:
		return "", false
	}
}

func normalizeOutgoingLevel(level string) string {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "warn", "warning":
		return "warning"
	case "info", "notice", "information":
		return "information"
	case "debug", "trace":
		return "debug"
	case "error", "fatal", "critical", "none":
		return "error"
	default:
		return "information"
	}
}

func invalidCommandResponse(method string) fiber.Map {
	return fiber.Map{
		"ErrorCode":        fiber.StatusBadRequest,
		"ErrorDetails":     method,
		"ErrorDescription": "1031: Invalid command.",
	}
}

func invalidParametersResponse() fiber.Map {
	return fiber.Map{
		"ErrorCode":        fiber.StatusBadRequest,
		"ErrorDetails":     "POST",
		"ErrorDescription": "1018: Invalid or missing parameters.",
	}
}

func successResponse() fiber.Map {
	return fiber.Map{
		"Code":      0,
		"Operation": "POST",
		"Details":   "Command completed.",
	}
}
