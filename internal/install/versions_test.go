package install

import (
	"os"
	"os/exec"
	"path/filepath"
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
	if s != "6.36.08 (05 Feb 2026) [latest] (recommended)" {
		t.Fatalf("unexpected string: %q", s)
	}

	v2 := ROOTVersion{Version: "6.34.10", Date: "27 Jun 2025"}
	s2 := v2.String()
	if s2 != "6.34.10 (27 Jun 2025)" {
		t.Fatalf("unexpected string: %q", s2)
	}
}

func TestExtractROOT(t *testing.T) {
	// Create a temporary directory for the test.
	tmpDir, err := os.MkdirTemp("", "extract-test")
	if err != nil {
		t.Fatalf("creating temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a dummy "root" directory with some content.
	srcDir := filepath.Join(tmpDir, "src")
	rootDir := filepath.Join(srcDir, "root")
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		t.Fatalf("creating root dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(rootDir, "this_is_root"), []byte("content"), 0o644); err != nil {
		t.Fatalf("writing content file: %v", err)
	}

	// Create a tarball of the "root" directory.
	tarballPath := filepath.Join(tmpDir, "root.tar.gz")
	cmd := exec.Command("tar", "-czf", tarballPath, "-C", srcDir, "root")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("creating tarball failed: %s: %v", out, err)
	}

	// Prepare destination directory.
	destParent := filepath.Join(tmpDir, "dest")
	if err := os.MkdirAll(destParent, 0o755); err != nil {
		t.Fatalf("creating dest dir: %v", err)
	}

	// Create a "ROOT" directory to simulate an existing installation (should be removed).
	existingROOT := filepath.Join(destParent, "ROOT")
	if err := os.Mkdir(existingROOT, 0o755); err != nil {
		t.Fatalf("creating existing ROOT: %v", err)
	}
	if err := os.WriteFile(filepath.Join(existingROOT, "old_file"), []byte("old"), 0o644); err != nil {
		t.Fatalf("writing old file: %v", err)
	}

	// Run ExtractROOT.
	if err := ExtractROOT(tarballPath, destParent); err != nil {
		t.Fatalf("ExtractROOT failed: %v", err)
	}

	// Verify "ROOT" exists and contains the new content.
	newROOT := filepath.Join(destParent, "ROOT")
	if _, err := os.Stat(newROOT); os.IsNotExist(err) {
		t.Fatalf("expected ROOT directory to exist")
	}
	if _, err := os.Stat(filepath.Join(newROOT, "this_is_root")); os.IsNotExist(err) {
		t.Fatalf("expected content file to exist in ROOT")
	}

	// Verify "root" directory does not exist (renamed).
	if _, err := os.Stat(filepath.Join(destParent, "root")); !os.IsNotExist(err) {
		t.Fatalf("expected root directory to be gone (renamed)")
	}

	// Verify old content is gone.
	if _, err := os.Stat(filepath.Join(newROOT, "old_file")); !os.IsNotExist(err) {
		t.Fatalf("expected old file to be removed")
	}
}
