package platform

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectShell(t *testing.T) {
	// Backup and restore environment
	originalShell := os.Getenv("SHELL")
	defer os.Setenv("SHELL", originalShell)

	os.Setenv("SHELL", "/bin/zsh")
	if got := DetectShell(); got != "zsh" {
		t.Errorf("expected zsh, got %q", got)
	}

	os.Setenv("SHELL", "/usr/local/bin/fish")
	if got := DetectShell(); got != "fish" {
		t.Errorf("expected fish, got %q", got)
	}

	os.Unsetenv("SHELL")
	if got := DetectShell(); got != "bash" {
		t.Errorf("expected fallback bash, got %q", got)
	}
}

func TestConfigureShell(t *testing.T) {
	// Mock userHomeDirFunc
	tmpHome, err := os.MkdirTemp("", "shell-test")
	if err != nil {
		t.Fatalf("creating temp home: %v", err)
	}
	defer os.RemoveAll(tmpHome)

	oldHomeFunc := userHomeDirFunc
	userHomeDirFunc = func() (string, error) {
		return tmpHome, nil
	}
	defer func() { userHomeDirFunc = oldHomeFunc }()

	tests := []struct {
		shell      string
		wantRC     string
		wantSource string
	}{
		{"bash", ".bashrc", "thisroot.sh"},
		{"zsh", ".zshrc", "thisroot.sh"},
		{"fish", ".config/fish/config.fish", "thisroot.fish"},
		{"csh", ".cshrc", "thisroot.csh"},
		{"sh", ".profile", "thisroot.sh"},
	}

	for _, tt := range tests {
		t.Run(tt.shell, func(t *testing.T) {
			rcPath, err := ConfigureShell(tt.shell)
			if err != nil {
				t.Fatalf("ConfigureShell failed: %v", err)
			}

			// Verify correct file path
			expectedPath := filepath.Join(tmpHome, tt.wantRC)
			if rcPath != expectedPath {
				t.Errorf("expected RC path %q, got %q", expectedPath, rcPath)
			}

			// Verify file content
			content, err := os.ReadFile(rcPath)
			if err != nil {
				t.Fatalf("reading RC file: %v", err)
			}
			if !strings.Contains(string(content), tt.wantSource) {
				t.Errorf("expected source command for %q in %s", tt.wantSource, rcPath)
			}
		})
	}
}

func TestConfigureShellUnsupported(t *testing.T) {
	_, err := ConfigureShell("unknownshell")
	if err == nil {
		t.Error("expected error for unsupported shell, got nil")
	}
}
