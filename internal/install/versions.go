package install

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const releasesURL = "https://root.cern/install/all_releases/"

// ROOTVersion represents a single ROOT release.
type ROOTVersion struct {
	Version  string // e.g. "6.36.08"
	Date     string // e.g. "06 Feb 2026"
	IsLatest bool
}

// String returns a display label for the version.
func (v ROOTVersion) String() string {
	s := v.Version
	if v.Date != "" {
		s += " (" + v.Date + ")"
	}
	if v.IsLatest {
		s = s + " [latest]"
	}
	return s
}

// releasePattern matches lines like:
//
//	Release 6.36.08 - 06 Feb 2026
//	Release 6.30/02 - 28 Nov 2023
var releasePattern = regexp.MustCompile(
	`Release\s+(\d+\.\d+[./]\d+)\s*-\s*(\d{1,2}\s+\w+\s+\d{4})`,
)

// latestPattern matches the "LATEST STABLE" link text that precedes the
// latest release on the page.
var latestPattern = regexp.MustCompile(
	`LATEST STABLE.*?Release\s+(\d+\.\d+[./]\d+)`,
)

// FetchROOTVersions scrapes the root.cern releases page and returns available
// stable ROOT versions, newest first. Release candidates (rc) are excluded.
func FetchROOTVersions() ([]ROOTVersion, error) {
	client := &http.Client{Timeout: 15 * time.Second}

	resp, err := client.Get(releasesURL)
	if err != nil {
		return nil, fmt.Errorf("fetching releases page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("releases page returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading releases page: %w", err)
	}

	return ParseROOTVersions(string(body))
}

// ParseROOTVersions extracts ROOT versions from the HTML/text content of the
// releases page. Exported so it can be tested with canned input.
// Only versions released within the last 8 months of the newest release are kept.
func ParseROOTVersions(content string) ([]ROOTVersion, error) {
	// Find the latest version label.
	latestVer := ""
	if m := latestPattern.FindStringSubmatch(content); len(m) >= 2 {
		latestVer = normalizeVersion(m[1])
	}

	matches := releasePattern.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil, fmt.Errorf("no releases found on page")
	}

	// Deduplicate: the page often lists the same version in multiple sections.
	seen := make(map[string]bool)
	var versions []ROOTVersion

	for _, m := range matches {
		ver := normalizeVersion(m[1])
		date := m[2]

		// Skip release candidates.
		if strings.Contains(strings.ToLower(ver), "rc") {
			continue
		}

		if seen[ver] {
			continue
		}
		seen[ver] = true

		versions = append(versions, ROOTVersion{
			Version:  ver,
			Date:     date,
			IsLatest: ver == latestVer,
		})
	}

	if len(versions) == 0 {
		return nil, fmt.Errorf("no stable releases found")
	}

	// Filter: keep only versions within 8 months of the newest release date.
	versions = filterByAge(versions, 8)

	return versions, nil
}

// dateLayouts are the formats the releases page uses for dates.
var dateLayouts = []string{
	"2 January 2006",
	"02 January 2006",
	"2 Jan 2006",
	"02 Jan 2006",
}

// parseReleaseDate attempts to parse a date string like "06 Feb 2026".
func parseReleaseDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	for _, layout := range dateLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// filterByAge keeps only versions whose release date is within `months`
// months of the newest release. Versions with unparseable dates are kept.
func filterByAge(versions []ROOTVersion, months int) []ROOTVersion {
	// Find the newest date.
	var newest time.Time
	for _, v := range versions {
		if t, ok := parseReleaseDate(v.Date); ok {
			if t.After(newest) {
				newest = t
			}
		}
	}
	if newest.IsZero() {
		return versions // can't determine cutoff, keep all
	}

	cutoff := newest.AddDate(0, -months, 0)

	var filtered []ROOTVersion
	for _, v := range versions {
		t, ok := parseReleaseDate(v.Date)
		if !ok {
			filtered = append(filtered, v) // keep unparseable dates
			continue
		}
		if !t.Before(cutoff) {
			filtered = append(filtered, v)
		}
	}
	return filtered
}

// normalizeVersion replaces 6.30/02 style with 6.30.02.
func normalizeVersion(v string) string {
	return strings.ReplaceAll(v, "/", ".")
}
