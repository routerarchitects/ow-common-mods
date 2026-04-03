package system

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	serviceVersion "github.com/routerarchitects/ra-common-mods/buildinfo"
	"github.com/routerarchitects/ra-common-mods/logger"
)

var processStartUnix int64

func init() {
	processStartUnix = time.Now().Unix()
}

type Routes struct {
	cfg Config
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

// New creates a route handler with the provided config.
func NewSubsytems(c Config) *Routes {
	return &Routes{cfg: c}
}

// RegisterFiberRoutes registers the system command routes on the receiver.
func (rt *Routes) RegisterFiberRoutes(r fiber.Router) {
	r.Get("/api/v1/system", rt.handleSystemGet)
	r.Post("/api/v1/system", rt.handleSystemPost)
	r.Options("/api/v1/system", handleSystemOptions)
}

func handleSystemOptions(c fiber.Ctx) error {
	c.Set("Allow", "GET,POST,OPTIONS")
	return c.SendStatus(fiber.StatusNoContent)
}

func (rt *Routes) handleSystemGet(c fiber.Ctx) error {
	switch strings.TrimSpace(c.Query("command")) {
	case "info":
		return c.JSON(rt.getSystemInfo())
	case "resources":
		return c.JSON(getResourceUsage())
	default:
		return c.Status(fiber.StatusBadRequest).JSON(invalidCommandResponse("GET"))
	}
}

func (rt *Routes) handleSystemPost(c fiber.Ctx) error {
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
			"list": logger.GetAllLevels(),
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

		if strings.EqualFold(tag, "all") {
			for name := range current {
				updates[name] = value
			}
			continue
		}

		// Keep success semantics even when the subsystem name is unknown.
		updates[tag] = value
	}

	if err := logger.UpdateSubsystemLevels(updates); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(invalidParametersResponse())
	}

	return c.JSON(successResponse())
}

func (rt *Routes) getSystemInfo() fiber.Map {
	hostname, _ := os.Hostname()

	certInfo := certInfoFromFile([]string{rt.cfg.serverCertificatePath, rt.cfg.websocketCertificatePath})

	version := serviceVersion.GetVersion()
	commitHash := serviceVersion.GetCommitHash()
	if version == "" {
		version = commitHash
	} else {
		version = fmt.Sprintf("%s-%s", version, commitHash)
	}

	return fiber.Map{
		"version":      version,
		"uptime":       time.Now().Unix() - processStartUnix,
		"start":        processStartUnix,
		"os":           runtime.GOOS,
		"processors":   runtime.NumCPU(),
		"hostname":     hostname,
		"UI":           rt.cfg.uiEndPoint,
		"certificates": certInfo,
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
			"value": strings.ToLower(levels[name]),
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

func certInfoFromFile(certificates []string) []map[string]interface{} {
	certificatesMap := make([]map[string]interface{}, 0, len(certificates))

	for _, path := range certificates {
		if path == "" {
			continue
		}
		pemData, err := os.ReadFile(path)
		if err != nil {
			certificatesMap = append(certificatesMap, map[string]interface{}{
				"filename":  fmt.Sprintf("file not found : %s", filepath.Base(path)),
				"expiresOn": 0,
			})
			continue
		}

		block, _ := pem.Decode(pemData)
		if block == nil {
			certificatesMap = append(certificatesMap, map[string]interface{}{
				"filename":  fmt.Sprintf("invalid cert file : %s", filepath.Base(path)),
				"expiresOn": 0,
			})
			continue
		}

		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			certificatesMap = append(certificatesMap, map[string]interface{}{
				"filename":  fmt.Sprintf("invalid cert file : %s", filepath.Base(path)),
				"expiresOn": 0,
			})
			continue
		}

		certificatesMap = append(certificatesMap, map[string]interface{}{
			"filename":  filepath.Base(path),
			"expiresOn": cert.NotAfter.Unix(),
		})
	}

	return certificatesMap
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
