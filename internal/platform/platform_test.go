package platform

import (
	"errors"
	"testing"
)

func TestParseDistroNameUbuntu(t *testing.T) {
	content := `NAME="Ubuntu"
VERSION="22.04.3 LTS (Jammy Jellyfish)"
ID=ubuntu
ID_LIKE=debian
`
	got := parseDistroName(content)
	if got != "Ubuntu" {
		t.Fatalf("expected Ubuntu, got %q", got)
	}
}

func TestParseDistroNameFedora(t *testing.T) {
	content := `NAME="Fedora Linux"
VERSION="39 (Workstation Edition)"
ID=fedora
`
	got := parseDistroName(content)
	if got != "Fedora Linux" {
		t.Fatalf("expected Fedora Linux, got %q", got)
	}
}

func TestParseDistroNameArch(t *testing.T) {
	content := `NAME="Arch Linux"
ID=arch
`
	got := parseDistroName(content)
	if got != "Arch Linux" {
		t.Fatalf("expected Arch Linux, got %q", got)
	}
}

func TestParseDistroNameEmpty(t *testing.T) {
	got := parseDistroName("")
	if got != "Linux" {
		t.Fatalf("expected Linux fallback, got %q", got)
	}
}

func TestParseDistroNameMalformed(t *testing.T) {
	content := `garbage=value
something else
`
	got := parseDistroName(content)
	if got != "Linux" {
		t.Fatalf("expected Linux fallback, got %q", got)
	}
}

func TestParseDistroNameNoQuotes(t *testing.T) {
	content := `NAME=Solus
ID=solus
`
	got := parseDistroName(content)
	if got != "Solus" {
		t.Fatalf("expected Solus, got %q", got)
	}
}

func TestDetectPackageManagerApt(t *testing.T) {
	lookup := func(name string) (string, error) {
		if name == "apt" {
			return "/usr/bin/apt", nil
		}
		return "", errors.New("not found")
	}
	got := detectPackageManager(lookup)
	if got != "apt" {
		t.Fatalf("expected apt, got %q", got)
	}
}

func TestDetectPackageManagerPacman(t *testing.T) {
	lookup := func(name string) (string, error) {
		if name == "pacman" {
			return "/usr/bin/pacman", nil
		}
		return "", errors.New("not found")
	}
	got := detectPackageManager(lookup)
	if got != "pacman" {
		t.Fatalf("expected pacman, got %q", got)
	}
}

func TestDetectPackageManagerNone(t *testing.T) {
	lookup := func(_ string) (string, error) {
		return "", errors.New("not found")
	}
	got := detectPackageManager(lookup)
	if got != "unknown" {
		t.Fatalf("expected unknown, got %q", got)
	}
}

func TestDetectPackageManagerPriority(t *testing.T) {
	// When both apt and dnf exist, apt should win (earlier in list).
	lookup := func(name string) (string, error) {
		if name == "apt" {
			return "/usr/bin/apt", nil
		}
		if name == "dnf" {
			return "/usr/bin/dnf", nil
		}
		return "", errors.New("not found")
	}
	got := detectPackageManager(lookup)
	if got != "apt" {
		t.Fatalf("expected apt (higher priority), got %q", got)
	}
}

func TestIsDistroFromContentUbuntu(t *testing.T) {
	content := `NAME="Ubuntu"
ID=ubuntu
ID_LIKE=debian
`
	if !isDistroFromContent(content, "ubuntu") {
		t.Fatal("expected ubuntu to match")
	}
	if !isDistroFromContent(content, "debian") {
		t.Fatal("expected debian to match via ID_LIKE")
	}
	if isDistroFromContent(content, "fedora") {
		t.Fatal("expected fedora not to match")
	}
}

func TestIsDistroFromContentCaseInsensitive(t *testing.T) {
	content := `NAME="Ubuntu"
ID=ubuntu
`
	if !isDistroFromContent(content, "Ubuntu") {
		t.Fatal("expected case-insensitive match")
	}
}

func TestIsDistroFromContentIDLikeMultiple(t *testing.T) {
	content := `NAME="Linux Mint"
ID=linuxmint
ID_LIKE="ubuntu debian"
`
	if !isDistroFromContent(content, "ubuntu") {
		t.Fatal("expected ubuntu match via ID_LIKE")
	}
	if !isDistroFromContent(content, "debian") {
		t.Fatal("expected debian match via ID_LIKE")
	}
}

func TestParseDistroID(t *testing.T) {
	content := `NAME="Ubuntu"
VERSION="22.04.3 LTS (Jammy Jellyfish)"
ID=ubuntu
ID_LIKE=debian
`
	if got := parseDistroID(content); got != "ubuntu" {
		t.Errorf("expected ubuntu, got %q", got)
	}
}

func TestParseDistroIDFallback(t *testing.T) {
	if got := parseDistroID(""); got != "linux" {
		t.Errorf("expected linux fallback, got %q", got)
	}
}

func TestParseDistroVersion(t *testing.T) {
	content := `NAME="Ubuntu"
VERSION_ID="22.04"
`
	if got := parseDistroVersion(content); got != "22.04" {
		t.Errorf("expected 22.04, got %q", got)
	}
}

func TestParseDistroVersionQuotes(t *testing.T) {
	content := `VERSION_ID="39"`
	if got := parseDistroVersion(content); got != "39" {
		t.Errorf("expected 39, got %q", got)
	}
}
