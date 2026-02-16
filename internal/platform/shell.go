package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DetectShell returns the user's shell name (e.g. "bash", "zsh", "fish").
// Falls back to "bash" if detection fails.
func DetectShell() string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		return "bash"
	}
	return filepath.Base(shell)
}

// userHomeDirFunc is a variable to allow mocking in tests.
var userHomeDirFunc = os.UserHomeDir

// ConfigureShell appends the ROOT source command to the user's shell configuration file.
// It returns the path to the modified RC file, or an error if the shell is unsupported.
func ConfigureShell(shell string) (string, error) {
	home, err := userHomeDirFunc()
	if err != nil {
		return "", fmt.Errorf("getting home dir: %w", err)
	}

	var rcFile string
	var sourceCommand string

	switch shell {
	case "fish":
		// fish config is usually in ~/.config/fish/config.fish
		rcFile = filepath.Join(home, ".config", "fish", "config.fish")
		sourceCommand = fmt.Sprintf("source %s/.local/ROOT/bin/thisroot.fish", home)
	case "csh", "tcsh":
		// csh/tcsh usually use ~/.cshrc or ~/.tcshrc
		// simpler to target .cshrc as it's often sourced by both or represents base config
		rcFile = filepath.Join(home, ".cshrc")
		sourceCommand = fmt.Sprintf("source %s/.local/ROOT/bin/thisroot.csh", home)
	case "zsh":
		rcFile = filepath.Join(home, ".zshrc")
		sourceCommand = fmt.Sprintf("source %s/.local/ROOT/bin/thisroot.sh", home)
	case "bash":
		rcFile = filepath.Join(home, ".bashrc")
		sourceCommand = fmt.Sprintf("source %s/.local/ROOT/bin/thisroot.sh", home)
	case "sh":
		rcFile = filepath.Join(home, ".profile")
		sourceCommand = fmt.Sprintf("source %s/.local/ROOT/bin/thisroot.sh", home)
	default:
		return "", fmt.Errorf("unsupported shell: %s. Please manually source thisroot.sh", shell)
	}

	// Ensure directory exists for config files (e.g. ~/.config/fish)
	if err := os.MkdirAll(filepath.Dir(rcFile), 0755); err != nil {
		return "", fmt.Errorf("creating config dir %s: %w", filepath.Dir(rcFile), err)
	}

	// Read existing content to check if already configured
	content, err := os.ReadFile(rcFile)
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("reading %s: %w", rcFile, err)
	}

	// Check if source command is already present
	// We check for the script name to be safe (e.g. just "thisroot.sh")
	scriptName := filepath.Base(strings.Fields(sourceCommand)[1])
	if strings.Contains(string(content), scriptName) {
		return rcFile, nil // Already configured
	}

	// Append configuration
	f, err := os.OpenFile(rcFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return "", fmt.Errorf("opening %s: %w", rcFile, err)
	}
	defer f.Close()

	if _, err := f.WriteString(fmt.Sprintf("\n# ROOT environment configuration\n%s\n", sourceCommand)); err != nil {
		return "", fmt.Errorf("writing to %s: %w", rcFile, err)
	}

	return rcFile, nil
}
