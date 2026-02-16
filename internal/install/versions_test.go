package install

import (
	"testing"
)

const sampleReleasesPage = `
<a href="/releases/release-63608/">LATEST STABLE Release 6.36.08 - 6 February 2026</a>
<h3>Version 6</h3>
<li><a href="/releases/release-63800/">Release 6.38.00 - 27 Nov 2025</a></li>
<li><a href="/releases/release-63800-rc1/">Release 6.38.00-rc1 - 05 Nov 2025</a></li>
<li><a href="/releases/release-63608/">Release 6.36.08 - 06 Feb 2026</a></li>
<li><a href="/releases/release-63606/">Release 6.36.06 - 26 Nov 2025</a></li>
<li><a href="/releases/release-63604/">Release 6.36.04 - 25 Aug 2025</a></li>
<li><a href="/releases/release-63602/">Release 6.36.02 - 09 Jul 2025</a></li>
<li><a href="/releases/release-63600/">Release 6.36.00 - 25 May 2025</a></li>
<li><a href="/releases/release-63600-rc1/">Release 6.36.00-rc1 - 23 Apr 2025</a></li>
<li><a href="/releases/release-63410/">Release 6.34.10 - 27 Jun 2025</a></li>
<li><a href="/releases/release-63002/">Release 6.30/02 - 28 Nov 2023</a></li>
`

func TestParseROOTVersionsBasic(t *testing.T) {
	versions, err := ParseROOTVersions(sampleReleasesPage)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(versions) == 0 {
		t.Fatal("expected at least one version")
	}

	// Should not contain any rc versions.
	for _, v := range versions {
		if v.Version == "6.38.00-rc1" || v.Version == "6.36.00-rc1" {
			t.Fatalf("rc version should be filtered out: %s", v.Version)
		}
	}
}

func TestParseROOTVersionsLatest(t *testing.T) {
	versions, err := ParseROOTVersions(sampleReleasesPage)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	foundLatest := false
	for _, v := range versions {
		if v.IsLatest {
			foundLatest = true
			if v.Version != "6.36.08" {
				t.Fatalf("expected latest to be 6.36.08, got %s", v.Version)
			}
		}
	}
	if !foundLatest {
		t.Fatal("expected one version marked as latest")
	}
}

func TestParseROOTVersionsDedup(t *testing.T) {
	versions, err := ParseROOTVersions(sampleReleasesPage)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	counts := make(map[string]int)
	for _, v := range versions {
		counts[v.Version]++
	}
	for ver, count := range counts {
		if count > 1 {
			t.Fatalf("version %s appears %d times (should be deduplicated)", ver, count)
		}
	}
}

func TestParseROOTVersionsAgeFilter(t *testing.T) {
	versions, err := ParseROOTVersions(sampleReleasesPage)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The newest release is 06 Feb 2026. With an 8-month window, the cutoff
	// is ~06 Jun 2025. So 6.30.02 (28 Nov 2023) should be filtered out,
	// and 6.36.00 (25 May 2025) should also be filtered out since it's before June 2025.
	for _, v := range versions {
		if v.Version == "6.30.02" {
			t.Fatalf("version 6.30.02 (Nov 2023) should be filtered out by 8-month cutoff")
		}
	}

	// 6.34.10 (27 Jun 2025) should still be present (within 8 months of Feb 2026).
	found := false
	for _, v := range versions {
		if v.Version == "6.34.10" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected 6.34.10 (Jun 2025) to be kept (within 8-month window)")
	}
}

func TestParseROOTVersionsEmpty(t *testing.T) {
	_, err := ParseROOTVersions("no releases here")
	if err == nil {
		t.Fatal("expected error for empty content")
	}
}

func TestROOTVersionString(t *testing.T) {
	v := ROOTVersion{Version: "6.36.08", Date: "06 Feb 2026", IsLatest: true}
	s := v.String()
	if s != "6.36.08 (06 Feb 2026) [latest]" {
		t.Fatalf("unexpected string: %q", s)
	}

	v2 := ROOTVersion{Version: "6.34.10", Date: "27 Jun 2025"}
	s2 := v2.String()
	if s2 != "6.34.10 (27 Jun 2025)" {
		t.Fatalf("unexpected string: %q", s2)
	}
}

func TestFilterByAge(t *testing.T) {
	versions := []ROOTVersion{
		{Version: "1.0", Date: "01 Jan 2026"},
		{Version: "0.9", Date: "01 Oct 2025"},
		{Version: "0.8", Date: "01 Mar 2025"},
		{Version: "0.7", Date: "01 Jan 2025"},
	}

	// 8-month cutoff from newest (Jan 2026) = May 2025.
	filtered := filterByAge(versions, 8)

	// 0.8 (Mar 2025) and 0.7 (Jan 2025) should be filtered out.
	for _, v := range filtered {
		if v.Version == "0.8" || v.Version == "0.7" {
			t.Fatalf("version %s should be filtered out", v.Version)
		}
	}
	// 1.0 and 0.9 should remain.
	if len(filtered) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(filtered))
	}
}

func TestParseReleaseDate(t *testing.T) {
	tests := []struct {
		input string
		ok    bool
	}{
		{"06 Feb 2026", true},
		{"6 February 2026", true},
		{"28 Nov 2023", true},
		{"garbage", false},
	}
	for _, tt := range tests {
		_, ok := parseReleaseDate(tt.input)
		if ok != tt.ok {
			t.Errorf("parseReleaseDate(%q): got ok=%v, want %v", tt.input, ok, tt.ok)
		}
	}
}
