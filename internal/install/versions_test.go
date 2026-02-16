package install

import (
	"testing"
	"time"
)

func TestTagToVersion(t *testing.T) {
	tests := []struct {
		tag  string
		want string
	}{
		{"v6-36-08", "6.36.08"},
		{"v6-32-22", "6.32.22"},
		{"v6-30-02", "6.30.02"},
		{"no-v-prefix", ""},
		{"v6-38-00-rc1", "6.38.00.rc1"},
	}
	for _, tt := range tests {
		got := tagToVersion(tt.tag)
		if got != tt.want {
			t.Errorf("tagToVersion(%q) = %q, want %q", tt.tag, got, tt.want)
		}
	}
}

func TestFormatDate(t *testing.T) {
	tests := []struct {
		rfc3339 string
		want    string
	}{
		{"2026-02-05T20:00:19Z", "05 Feb 2026"},
		{"2025-11-27T10:00:00Z", "27 Nov 2025"},
	}
	for _, tt := range tests {
		parsed, err := time.Parse(time.RFC3339, tt.rfc3339)
		if err != nil {
			t.Fatalf("failed to parse %q: %v", tt.rfc3339, err)
		}
		got := formatDate(parsed)
		if got != tt.want {
			t.Errorf("formatDate(%q) = %q, want %q", tt.rfc3339, got, tt.want)
		}
	}
}

func TestParseDisplayDate(t *testing.T) {
	tests := []struct {
		input string
		ok    bool
	}{
		{"05 Feb 2026", true},
		{"27 Nov 2025", true},
		{"garbage", false},
	}
	for _, tt := range tests {
		_, ok := parseDisplayDate(tt.input)
		if ok != tt.ok {
			t.Errorf("parseDisplayDate(%q): got ok=%v, want %v", tt.input, ok, tt.ok)
		}
	}
}

func TestROOTVersionString(t *testing.T) {
	v := ROOTVersion{Version: "6.36.08", Date: "05 Feb 2026", IsLatest: true}
	s := v.String()
	if s != "6.36.08 (05 Feb 2026) [latest]" {
		t.Fatalf("unexpected string: %q", s)
	}

	v2 := ROOTVersion{Version: "6.34.10", Date: "27 Jun 2025"}
	s2 := v2.String()
	if s2 != "6.34.10 (27 Jun 2025)" {
		t.Fatalf("unexpected string: %q", s2)
	}
}
