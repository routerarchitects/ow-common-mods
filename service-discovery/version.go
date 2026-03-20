package servicediscovery

import (
	"regexp"
	"strconv"
	"strings"
)

var leadingSemver = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)`)

type semver3 struct {
	major int
	minor int
	patch int
	ok    bool
}

func parseLeadingSemver(s string) semver3 {
	s = strings.TrimSpace(s)
	m := leadingSemver.FindStringSubmatch(s)
	if len(m) != 4 {
		return semver3{ok: false}
	}
	maj, err := strconv.Atoi(m[1])
	if err != nil {
		return semver3{ok: false}
	}
	min, err := strconv.Atoi(m[2])
	if err != nil {
		return semver3{ok: false}
	}
	pat, err := strconv.Atoi(m[3])
	if err != nil {
		return semver3{ok: false}
	}
	return semver3{major: maj, minor: min, patch: pat, ok: true}
}

// compareVersionLoose compares two version strings.
//
// It first compares leading "x.y.z" semver triples if present in both.
// If semver cannot be parsed from either, it falls back to lexical compare.
//
// Returns:
//   -1 if a < b
//    0 if a == b
//    1 if a > b
func compareVersionLoose(a, b string) int {
	sa := parseLeadingSemver(a)
	sb := parseLeadingSemver(b)
	if sa.ok && sb.ok {
		if sa.major != sb.major {
			if sa.major < sb.major {
				return -1
			}
			return 1
		}
		if sa.minor != sb.minor {
			if sa.minor < sb.minor {
				return -1
			}
			return 1
		}
		if sa.patch != sb.patch {
			if sa.patch < sb.patch {
				return -1
			}
			return 1
		}
		return 0
	}

	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == b {
		return 0
	}
	if a < b {
		return -1
	}
	return 1
}
