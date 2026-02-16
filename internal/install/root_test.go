package install

import (
	"context"
	"errors"
	"io"
	"testing"
)

type mockRunner struct {
	lookups  map[string][]lookupResponse
	commands []commandCall
}

type lookupResponse struct {
	path string
	err  error
}

type commandCall struct {
	name string
	args []string
}

func (m *mockRunner) LookPath(file string) (string, error) {
	entries := m.lookups[file]
	if len(entries) == 0 {
		return "", errors.New("not found")
	}
	current := entries[0]
	m.lookups[file] = entries[1:]
	return current.path, current.err
}

func (m *mockRunner) Run(_ context.Context, name string, args ...string) error {
	m.commands = append(m.commands, commandCall{name: name, args: args})
	return nil
}

type fixedPrompter struct {
	choice int
}

func (f fixedPrompter) Choose(_ string, _ []string) (int, error) {
	return f.choice, nil
}

func TestInstallOnMacOSUsesHomebrewWhenOnlyBrewExists(t *testing.T) {
	runner := &mockRunner{
		lookups: map[string][]lookupResponse{
			"brew": {{path: "/opt/homebrew/bin/brew"}},
			"port": {{err: errors.New("not found")}},
		},
	}
	installer := NewRootInstaller(runner, fixedPrompter{}, io.Discard)

	if err := installer.installOnMacOS(context.Background()); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(runner.commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(runner.commands))
	}
	if runner.commands[0].name != "/opt/homebrew/bin/brew" {
		t.Fatalf("expected brew install command, got %s", runner.commands[0].name)
	}
}

func TestInstallOnMacOSUsesMacPortsWhenOnlyPortExists(t *testing.T) {
	runner := &mockRunner{
		lookups: map[string][]lookupResponse{
			"brew": {{err: errors.New("not found")}},
			"port": {{path: "/opt/local/bin/port"}},
		},
	}
	installer := NewRootInstaller(runner, fixedPrompter{}, io.Discard)

	if err := installer.installOnMacOS(context.Background()); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(runner.commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(runner.commands))
	}
	if runner.commands[0].name != "sudo" {
		t.Fatalf("expected sudo command, got %s", runner.commands[0].name)
	}
}

func TestInstallOnMacOSAsksWhenBothManagersExist(t *testing.T) {
	runner := &mockRunner{
		lookups: map[string][]lookupResponse{
			"brew": {{path: "/opt/homebrew/bin/brew"}},
			"port": {{path: "/opt/local/bin/port"}},
		},
	}
	installer := NewRootInstaller(runner, fixedPrompter{choice: 1}, io.Discard)

	if err := installer.installOnMacOS(context.Background()); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(runner.commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(runner.commands))
	}
	if runner.commands[0].name != "sudo" {
		t.Fatalf("expected macports path via sudo, got %s", runner.commands[0].name)
	}
}

func TestInstallOnMacOSUsesPreferredManagerWhenBothExist(t *testing.T) {
	runner := &mockRunner{
		lookups: map[string][]lookupResponse{
			"brew": {{path: "/opt/homebrew/bin/brew"}},
			"port": {{path: "/opt/local/bin/port"}},
		},
	}
	installer := NewRootInstaller(runner, fixedPrompter{choice: 1}, io.Discard)
	installer.SetPreferredManager(ManagerHomebrew)

	if err := installer.installOnMacOS(context.Background()); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(runner.commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(runner.commands))
	}
	if runner.commands[0].name != "/opt/homebrew/bin/brew" {
		t.Fatalf("expected brew command when preferred, got %s", runner.commands[0].name)
	}
}

func TestInstallOnMacOSInstallsHomebrewWhenNoManagerExists(t *testing.T) {
	runner := &mockRunner{
		lookups: map[string][]lookupResponse{
			"brew": {
				{err: errors.New("not found")},
				{path: "/opt/homebrew/bin/brew"},
			},
			"port": {{err: errors.New("not found")}},
		},
	}
	installer := NewRootInstaller(runner, fixedPrompter{}, io.Discard)

	if err := installer.installOnMacOS(context.Background()); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(runner.commands) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(runner.commands))
	}

	if runner.commands[0].name != "/bin/bash" {
		t.Fatalf("expected homebrew bootstrap, got %s", runner.commands[0].name)
	}
	if runner.commands[1].name != "/opt/homebrew/bin/brew" {
		t.Fatalf("expected brew install root, got %s", runner.commands[1].name)
	}
}

func TestNewRootInstallerRequiresPrompterInterface(t *testing.T) {
	var _ Prompter = fixedPrompter{}
}
