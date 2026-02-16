package install

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	// GitHub API endpoint for ROOT releases (paginated, newest first).
	ghReleasesURL = "https://api.github.com/repos/root-project/root/releases?per_page=100"
	// GitHub API endpoint for the release tagged "latest".
	ghLatestURL = "https://api.github.com/repos/root-project/root/releases/latest"
)

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
		s = s + " [latest] (recommended)"
	}
	return s
}

// ghRelease is the subset of fields we need from the GitHub API response.
type ghRelease struct {
	TagName     string `json:"tag_name"`
	PublishedAt string `json:"published_at"`
	Prerelease  bool   `json:"prerelease"`
	Draft       bool   `json:"draft"`
}

// FetchROOTVersions fetches available ROOT versions from the GitHub releases
// API. Returns stable versions released within 8 months of the latest, newest
// first. The version marked "latest" on GitHub is flagged with IsLatest.
func FetchROOTVersions() ([]ROOTVersion, error) {
	client := &http.Client{Timeout: 15 * time.Second}

	// 1. Get the tag marked as "latest" on GitHub.
	latestTag, err := fetchLatestTag(client)
	if err != nil {
		return nil, err
	}

	// 2. Get all releases.
	releases, err := fetchAllReleases(client)
	if err != nil {
		return nil, err
	}

	// 3. Convert to ROOTVersion, applying filters.
	latestVer := tagToVersion(latestTag)
	var versions []ROOTVersion
	var newest time.Time

	for _, r := range releases {
		if r.Draft || r.Prerelease {
			continue
		}
		ver := tagToVersion(r.TagName)
		if ver == "" {
			continue
		}
		// Skip release candidates (tag might not set prerelease flag).
		if strings.Contains(strings.ToLower(r.TagName), "rc") {
			continue
		}

		pubDate, _ := time.Parse(time.RFC3339, r.PublishedAt)
		if pubDate.After(newest) {
			newest = pubDate
		}

		versions = append(versions, ROOTVersion{
			Version:  ver,
			Date:     formatDate(pubDate),
			IsLatest: ver == latestVer,
		})
	}

	if len(versions) == 0 {
		return nil, fmt.Errorf("no stable ROOT releases found")
	}

	// 4. Apply 8-month cutoff from the newest release.
	if !newest.IsZero() {
		cutoff := newest.AddDate(0, -8, 0)
		var filtered []ROOTVersion
		for _, v := range versions {
			t, ok := parseDisplayDate(v.Date)
			if !ok || !t.Before(cutoff) {
				filtered = append(filtered, v)
			}
		}
		versions = filtered
	}

	return versions, nil
}

// fetchLatestTag returns the tag_name of the release flagged as "latest".
func fetchLatestTag(client *http.Client) (string, error) {
	req, _ := http.NewRequest("GET", ghLatestURL, nil)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching latest release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub latest release returned status %d", resp.StatusCode)
	}

	var r ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", fmt.Errorf("decoding latest release: %w", err)
	}
	return r.TagName, nil
}

// fetchAllReleases returns all releases from the GitHub API (up to 100).
func fetchAllReleases(client *http.Client) ([]ghRelease, error) {
	req, _ := http.NewRequest("GET", ghReleasesURL, nil)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching releases: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub releases returned status %d", resp.StatusCode)
	}

	var releases []ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("decoding releases: %w", err)
	}
	return releases, nil
}

// tagToVersion converts a GitHub tag like "v6-36-08" to "6.36.08".
// Returns "" for tags that don't match the expected pattern.
func tagToVersion(tag string) string {
	t := strings.TrimPrefix(tag, "v")
	if t == tag {
		return "" // no "v" prefix, not a version tag
	}
	return strings.ReplaceAll(t, "-", ".")
}

// formatDate formats a time.Time to "02 Jan 2006" for display.
func formatDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("02 Jan 2006")
}

// parseDisplayDate parses a date string formatted by formatDate.
func parseDisplayDate(s string) (time.Time, bool) {
	t, err := time.Parse("02 Jan 2006", s)
	return t, err == nil
}
