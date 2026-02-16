package install

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

const homebrewInstallCommand = `$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)`

type ManagerChoice string

const (
	ManagerAuto     ManagerChoice = "auto"
	ManagerHomebrew ManagerChoice = "brew"
	ManagerMacPorts ManagerChoice = "port"
)

// Prompter handles interactive selections in non-TUI fallback flows.
type Prompter interface {
	Choose(label string, options []string) (int, error)
}

type MacManagerDetection struct {
	BrewExists bool
	PortExists bool
	BrewPath   string
	PortPath   string
}

// Runner encapsulates command discovery and execution.
type Runner interface {
	LookPath(file string) (string, error)
	Run(ctx context.Context, name string, args ...string) error
}

// DefaultRunner executes commands with stdio attached.
type DefaultRunner struct{}

func (DefaultRunner) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

func (DefaultRunner) Run(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// RootInstaller installs ROOT using the best available platform method.
type RootInstaller struct {
	runner           Runner
	prompter         Prompter
	out              io.Writer
	preferredManager ManagerChoice
}

func NewRootInstaller(runner Runner, prompter Prompter, out io.Writer) *RootInstaller {
	return &RootInstaller{
		runner:           runner,
		prompter:         prompter,
		out:              out,
		preferredManager: ManagerAuto,
	}
}

func (i *RootInstaller) SetPreferredManager(choice ManagerChoice) {
	switch choice {
	case ManagerHomebrew, ManagerMacPorts:
		i.preferredManager = choice
	default:
		i.preferredManager = ManagerAuto
	}
}

func (i *RootInstaller) DetectMacManagers() MacManagerDetection {
	brewPath, brewErr := i.runner.LookPath("brew")
	portPath, portErr := i.runner.LookPath("port")

	return MacManagerDetection{
		BrewExists: brewErr == nil && brewPath != "",
		PortExists: portErr == nil && portPath != "",
		BrewPath:   brewPath,
		PortPath:   portPath,
	}
}

func (i *RootInstaller) Install(ctx context.Context) error {
	switch runtime.GOOS {
	case "darwin":
		return i.installOnMacOS(ctx)
	case "linux":
		if isUbuntu() {
			return errors.New("ubuntu support is planned but not implemented yet")
		}
		return errors.New("linux detected, but only ubuntu is in scope for the next step")
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

func (i *RootInstaller) installOnMacOS(ctx context.Context) error {
	detected := i.DetectMacManagers()
	brewExists := detected.BrewExists
	portExists := detected.PortExists
	brewPath := detected.BrewPath
	portPath := detected.PortPath

	switch {
	case brewExists && !portExists:
		fmt.Fprintln(i.out, "Detected Homebrew. Installing ROOT with Homebrew...")
		return i.installWithBrew(ctx, brewPath)
	case !brewExists && portExists:
		fmt.Fprintln(i.out, "Detected MacPorts. Installing ROOT with MacPorts...")
		return i.installWithMacPorts(ctx, portPath)
	case brewExists && portExists:
		if i.preferredManager == ManagerHomebrew {
			fmt.Fprintln(i.out, "Both managers found. Using preferred manager: Homebrew.")
			return i.installWithBrew(ctx, brewPath)
		}
		if i.preferredManager == ManagerMacPorts {
			fmt.Fprintln(i.out, "Both managers found. Using preferred manager: MacPorts.")
			return i.installWithMacPorts(ctx, portPath)
		}
		if i.prompter == nil {
			return errors.New("both Homebrew and MacPorts found, but no selection method is configured")
		}

		choice, err := i.prompter.Choose("Both Homebrew and MacPorts were found. Pick one:", []string{
			"Homebrew",
			"MacPorts",
		})
		if err != nil {
			return err
		}

		if choice == 0 {
			return i.installWithBrew(ctx, brewPath)
		}
		if choice == 1 {
			return i.installWithMacPorts(ctx, portPath)
		}
		return errors.New("invalid package manager selection")
	default:
		fmt.Fprintln(i.out, "Neither Homebrew nor MacPorts was found.")
		fmt.Fprintln(i.out, "Installing Homebrew first...")
		if err := i.installHomebrew(ctx); err != nil {
			return fmt.Errorf("homebrew installation failed: %w", err)
		}

		resolvedBrew, err := i.resolveBrewPath()
		if err != nil {
			return err
		}
		return i.installWithBrew(ctx, resolvedBrew)
	}
}

func (i *RootInstaller) installWithBrew(ctx context.Context, brewPath string) error {
	fmt.Fprintln(i.out, "Running:", brewPath, "install", "root")
	return i.runner.Run(ctx, brewPath, "install", "root")
}

func (i *RootInstaller) installWithMacPorts(ctx context.Context, portPath string) error {
	fmt.Fprintln(i.out, "Running: sudo", portPath, "install", "root6")
	return i.runner.Run(ctx, "sudo", portPath, "install", "root6")
}

func (i *RootInstaller) installHomebrew(ctx context.Context) error {
	fmt.Fprintln(i.out, "Running: /bin/bash -c", homebrewInstallCommand)
	return i.runner.Run(ctx, "/bin/bash", "-c", homebrewInstallCommand)
}

func (i *RootInstaller) resolveBrewPath() (string, error) {
	brewPath, err := i.runner.LookPath("brew")
	if err == nil && brewPath != "" {
		return brewPath, nil
	}

	for _, candidate := range []string{"/opt/homebrew/bin/brew", "/usr/local/bin/brew"} {
		if _, statErr := os.Stat(candidate); statErr == nil {
			return candidate, nil
		}
	}

	return "", errors.New("homebrew installed but `brew` was not found in PATH; rerun your shell and try again")
}

func isUbuntu() bool {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return false
	}

	lower := strings.ToLower(string(data))
	return strings.Contains(lower, "id=ubuntu") || strings.Contains(lower, "id_like=ubuntu")
}
