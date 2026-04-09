package platform

import (
	"os"
	"os/exec"
	"strings"
)

// DistroName returns the Linux distribution name by parsing /etc/os-release.
// Falls back to "Linux" if the file is missing or the NAME field is absent.
func DistroName() string {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return "Linux"
	}
	return parseDistroName(string(data))
}

// DistroID returns the ID from /etc/os-release (e.g. "ubuntu", "fedora").
// Returns "linux" if unavailable.
func DistroID() string {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return "linux"
	}
	return parseDistroID(string(data))
}

func parseDistroID(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ID=") {
			val := strings.TrimPrefix(line, "ID=")
			val = strings.Trim(val, `"`)
			return strings.TrimSpace(val)
		}
	}
	return "linux"
}

// parseDistroName extracts the NAME field from os-release content.
func parseDistroName(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "NAME=") {
			val := strings.TrimPrefix(line, "NAME=")
			val = strings.Trim(val, `"`)
			val = strings.TrimSpace(val)
			if val != "" {
				return val
			}
		}
	}
	return "Linux"
}

// DistroVersion returns the VERSION_ID from /etc/os-release (e.g. "24.04").
// Returns "" if unavailable.
func DistroVersion() string {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return ""
	}
	return parseDistroVersion(string(data))
}

func parseDistroVersion(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "VERSION_ID=") {
			val := strings.TrimPrefix(line, "VERSION_ID=")
			val = strings.Trim(val, `"`)
			return strings.TrimSpace(val)
		}
	}
	return ""
}

// knownManagers is the ordered list of package managers to probe for.
var knownManagers = []string{
	"apt",
	"dnf",
	"yum",
	"pacman",
	"zypper",
	"apk",
	"emerge",
	"xbps-install",
	"nix-env",
	"eopkg",
}

// PackageManager returns the name of the first detected package manager,
// or "unknown" if none is found.
func PackageManager() string {
	return detectPackageManager(exec.LookPath)
}

func detectPackageManager(lookPath func(string) (string, error)) string {
	for _, mgr := range knownManagers {
		if path, err := lookPath(mgr); err == nil && path != "" {
			return mgr
		}
	}
	return "unknown"
}

// IsDistro reports whether the running Linux distribution matches the given
// name (case-insensitive). It reads /etc/os-release and checks both ID and
// ID_LIKE fields.
func IsDistro(name string) bool {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return false
	}
	return isDistroFromContent(string(data), name)
}

func isDistroFromContent(content, name string) bool {
	target := strings.ToLower(name)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ID=") {
			val := strings.TrimPrefix(line, "ID=")
			val = strings.Trim(val, `"`)
			if strings.ToLower(val) == target {
				return true
			}
		}
		if strings.HasPrefix(line, "ID_LIKE=") {
			val := strings.TrimPrefix(line, "ID_LIKE=")
			val = strings.Trim(val, `"`)
			for _, part := range strings.Fields(val) {
				if strings.ToLower(part) == target {
					return true
				}
			}
		}
	}
	return false
}
