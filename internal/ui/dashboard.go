package ui

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode"

	"hepctl/internal/install"
	"hepctl/internal/platform"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type Action int

const (
	ActionNone Action = iota
	ActionInstallROOT
	ActionQuit
)

var installablePackages = []string{
	"root",
}

type installEvent struct {
	line string
	done bool
	err  error
}

type installEventMsg installEvent

type sudoResultMsg struct {
	err error
}

type versionsFetchedMsg struct {
	versions []install.ROOTVersion
	err      error
}

type tickMsg struct{}

type dashboardModel struct {
	cwd          string
	branch       string
	platform     string
	cpuArch      string
	managerState string
	lastCommand  string

	width  int
	height int

	input  []rune
	cursor int

	suggestIndex int

	status      string
	statusError bool

	awaitingManager bool
	selectedManager install.ManagerChoice

	passwordMode  bool
	passwordInput []rune

	versionSelectMode   bool
	availableVersions   []install.ROOTVersion
	versionCursor       int
	versionFetching     bool
	selectedROOTVersion string

	running bool
	spin    int
	events  chan installEvent
	logs    []string

	action Action
}

func RunDashboard() error {
	cwd := currentDirName()
	m := dashboardModel{
		cwd:             cwd,
		branch:          detectGitBranch(),
		platform:        platformLabel(),
		cpuArch:         runtime.GOARCH,
		managerState:    detectManagerState(),
		lastCommand:     "-",
		action:          ActionNone,
		status:          "Type a command and press Enter.",
		input:           []rune(""),
		cursor:          0,
		suggestIndex:    0,
		selectedManager: install.ManagerAuto,
		logs:            []string{},
	}

	finalModel, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	if err != nil {
		return err
	}

	final, ok := finalModel.(dashboardModel)
	if !ok {
		return errors.New("unexpected dashboard model type")
	}

	if final.action == ActionQuit || final.action == ActionNone {
		return nil
	}
	return nil
}

func (m dashboardModel) Init() tea.Cmd {
	return nil
}

func (m dashboardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch typed := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = typed.Width
		m.height = typed.Height
	case tickMsg:
		if m.running {
			m.spin = (m.spin + 1) % 4
			return m, tickCmd()
		}
	case sudoResultMsg:
		if typed.err != nil {
			m.status = "Authentication failed. Try again with `install root`."
			m.statusError = true
			return m, nil
		}
		m.status = "Authenticated. Starting ROOT installation..."
		m.statusError = false
		m.logs = append(m.logs, "[ok] sudo credentials cached")
		return m, m.startInstallRoot()
	case versionsFetchedMsg:
		m.versionFetching = false
		if typed.err != nil {
			m.status = "Failed to fetch versions: " + typed.err.Error()
			m.statusError = true
			m.logs = append(m.logs, "[error] "+typed.err.Error())
			return m, nil
		}
		if len(typed.versions) == 0 {
			m.status = "No ROOT versions found."
			m.statusError = true
			return m, nil
		}
		m.availableVersions = typed.versions
		m.versionCursor = 0
		m.versionSelectMode = true
		m.status = "Select a ROOT version."
		m.statusError = false
		m.logs = append(m.logs, "[ok] fetched "+fmt.Sprintf("%d", len(typed.versions))+" versions")
		return m, nil
	case installEventMsg:
		ev := installEvent(typed)
		if ev.line != "" {
			m.logs = append(m.logs, ev.line)
		}
		if ev.done {
			m.running = false
			if ev.err != nil {
				m.status = "Install failed: " + ev.err.Error()
				m.statusError = true
				m.logs = append(m.logs, "[error] "+ev.err.Error())
			} else {
				m.status = "ROOT installation completed."
				m.statusError = false
				m.logs = append(m.logs, "[ok] ROOT installation finished")
			}
			m.events = nil
			m.selectedManager = install.ManagerAuto
			return m, nil
		}
		if m.events != nil {
			return m, waitForInstallEvent(m.events)
		}
	case tea.KeyMsg:
		// Version selection mode: intercept keys.
		if m.versionSelectMode {
			switch typed.String() {
			case "up", "k":
				if m.versionCursor > 0 {
					m.versionCursor--
				}
			case "down", "j":
				if m.versionCursor < len(m.availableVersions)-1 {
					m.versionCursor++
				}
			case "enter":
				selected := m.availableVersions[m.versionCursor]
				m.versionSelectMode = false
				m.selectedROOTVersion = selected.Version
				m.availableVersions = nil
				m.status = "Selected ROOT " + selected.Version + ". (Install step coming soon.)"
				m.statusError = false
				m.logs = append(m.logs, "[ok] selected ROOT "+selected.Version)
			case "esc", "ctrl+c":
				m.versionSelectMode = false
				m.availableVersions = nil
				m.status = "Version selection canceled."
				m.statusError = false
			}
			return m, nil
		}

		// Password mode: handle input separately.
		if m.passwordMode {
			switch typed.String() {
			case "enter":
				password := string(m.passwordInput)
				// Clear password from model immediately.
				for i := range m.passwordInput {
					m.passwordInput[i] = 0
				}
				m.passwordInput = nil
				m.passwordMode = false
				m.status = "Authenticating..."
				m.statusError = false
				return m, trySudoAuth(password)
			case "esc", "ctrl+c":
				for i := range m.passwordInput {
					m.passwordInput[i] = 0
				}
				m.passwordInput = nil
				m.passwordMode = false
				m.status = "Password entry canceled."
				m.statusError = false
				return m, nil
			case "backspace":
				if len(m.passwordInput) > 0 {
					m.passwordInput = m.passwordInput[:len(m.passwordInput)-1]
				}
			default:
				if len(typed.Runes) > 0 {
					m.passwordInput = append(m.passwordInput, typed.Runes...)
				}
			}
			return m, nil
		}

		suggestions := m.installSuggestions()
		switch typed.String() {
		case "left":
			if !m.running && m.cursor > 0 {
				m.cursor--
			}
		case "right":
			if !m.running && m.cursor < len(m.input) {
				m.cursor++
			}
		case "home", "ctrl+a":
			if !m.running {
				m.cursor = 0
			}
		case "end", "ctrl+e":
			if !m.running {
				m.cursor = len(m.input)
			}
		case "backspace":
			if !m.running && m.cursor > 0 {
				m.input = append(m.input[:m.cursor-1], m.input[m.cursor:]...)
				m.cursor--
				m.suggestIndex = 0
			}
		case "delete":
			if !m.running && m.cursor < len(m.input) {
				m.input = append(m.input[:m.cursor], m.input[m.cursor+1:]...)
				m.suggestIndex = 0
			}
		case "up":
			if !m.running && !m.awaitingManager && len(suggestions) > 0 {
				m.suggestIndex--
				if m.suggestIndex < 0 {
					m.suggestIndex = len(suggestions) - 1
				}
			}
		case "down":
			if !m.running && !m.awaitingManager && len(suggestions) > 0 {
				m.suggestIndex++
				if m.suggestIndex >= len(suggestions) {
					m.suggestIndex = 0
				}
			}
		case "tab":
			if !m.running && !m.awaitingManager {
				if len(suggestions) > 0 {
					selected := m.suggestIndex
					if selected < 0 || selected >= len(suggestions) {
						selected = 0
					}
					completed := "install " + suggestions[selected]
					m.input = []rune(completed)
					m.cursor = len(m.input)
					m.suggestIndex = 0
					m.status = "Autocomplete: " + suggestions[selected]
					m.statusError = false
				}
			}
		case "enter":
			if m.running {
				return m, nil
			}
			cmd := strings.TrimSpace(string(m.input))
			m.input = []rune{}
			m.cursor = 0
			m.suggestIndex = 0
			next := m.handleCommand(cmd)
			return m, next
		case "esc", "ctrl+c":
			if m.running {
				m.status = "Install is still running. Wait for completion or close this terminal."
				m.statusError = true
				return m, nil
			}
			m.action = ActionQuit
			return m, tea.Quit
		default:
			if !m.running && !m.awaitingManager && len(suggestions) > 0 {
				if typed.String() == "j" {
					m.suggestIndex++
					if m.suggestIndex >= len(suggestions) {
						m.suggestIndex = 0
					}
					return m, nil
				}
				if typed.String() == "k" {
					m.suggestIndex--
					if m.suggestIndex < 0 {
						m.suggestIndex = len(suggestions) - 1
					}
					return m, nil
				}
			}
			if !m.running && len(typed.Runes) > 0 {
				m.input = insertAt(m.input, m.cursor, typed.Runes)
				m.cursor += len(typed.Runes)
				m.suggestIndex = 0
			}
		}
	}

	return m, nil
}

func (m *dashboardModel) handleCommand(raw string) tea.Cmd {
	cmd := normalizeCommand(raw)
	if strings.TrimSpace(raw) != "" {
		m.lastCommand = cmd
	}

	if m.awaitingManager {
		switch cmd {
		case "brew", "use brew", "manager brew":
			m.awaitingManager = false
			m.selectedManager = install.ManagerHomebrew
			m.status = "Using Homebrew. Starting ROOT installation..."
			m.statusError = false
			return m.startInstallRoot()
		case "port", "use port", "manager port":
			m.awaitingManager = false
			m.selectedManager = install.ManagerMacPorts
			m.status = "Using MacPorts. Starting ROOT installation..."
			m.statusError = false
			return m.startInstallRoot()
		case "cancel", "quit", "exit":
			m.awaitingManager = false
			m.selectedManager = install.ManagerAuto
			m.status = "Canceled manager selection."
			m.statusError = false
			return nil
		default:
			m.status = "Choose manager: type `brew` or `port` (or `cancel`)."
			m.statusError = true
			return nil
		}
	}

	switch cmd {
	case "":
		m.status = "Type `install root`, `help`, or `quit`."
		m.statusError = false
		return nil
	case "install root", "root install":
		if runtime.GOOS == "darwin" {
			probe := install.NewRootInstaller(install.DefaultRunner{}, nil, io.Discard)
			detected := probe.DetectMacManagers()
			if detected.BrewExists && detected.PortExists {
				m.awaitingManager = true
				m.status = "Both Homebrew and MacPorts found. Type `brew` or `port`."
				m.statusError = false
				return nil
			}
		}
		// On Ubuntu/Debian, ROOT isn't in the repos — offer version selection.
		if runtime.GOOS == "linux" && needsVersionSelection() {
			m.versionFetching = true
			m.status = "Fetching available ROOT versions..."
			m.statusError = false
			m.logs = append(m.logs, "$ fetching root.cern releases...")
			return fetchVersionsCmd()
		}
		// On Linux, install commands use sudo. Pre-authenticate if needed.
		if runtime.GOOS == "linux" {
			if needsSudoAuth() {
				m.selectedManager = install.ManagerAuto
				m.passwordMode = true
				m.passwordInput = nil
				m.status = "Enter sudo password to continue."
				m.statusError = false
				return nil
			}
		}
		m.selectedManager = install.ManagerAuto
		m.status = "Starting ROOT installation..."
		m.statusError = false
		return m.startInstallRoot()
	case "help":
		m.status = "Commands: install root | help | quit"
		m.statusError = false
		return nil
	case "quit", "exit":
		m.action = ActionQuit
		return tea.Quit
	default:
		m.status = "Unknown command: " + raw
		m.statusError = true
		return nil
	}
}

func (m *dashboardModel) startInstallRoot() tea.Cmd {
	if m.running {
		m.status = "Installation already in progress."
		m.statusError = true
		return nil
	}

	m.running = true
	m.logs = append(m.logs, "$ hepctl install root")
	m.spin = 0
	m.events = make(chan installEvent, 256)

	manager := m.selectedManager
	events := m.events

	go runInstallRoot(events, manager)
	return tea.Batch(waitForInstallEvent(events), tickCmd())
}

func runInstallRoot(events chan<- installEvent, manager install.ManagerChoice) {
	send := func(line string) {
		if strings.TrimSpace(line) == "" {
			return
		}
		events <- installEvent{line: line}
	}

	writer := newEventWriter(send)
	runner := &streamingRunner{emit: send}
	inst := install.NewRootInstaller(runner, nil, writer)
	inst.SetPreferredManager(manager)

	err := inst.Install(context.Background())
	writer.Flush()
	events <- installEvent{done: true, err: err}
	close(events)
}

func waitForInstallEvent(events <-chan installEvent) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-events
		if !ok {
			return installEventMsg(installEvent{done: true})
		}
		return installEventMsg(ev)
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(140*time.Millisecond, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

func (m dashboardModel) View() string {
	doc := lipgloss.NewStyle()

	width := 112
	if m.width > 0 {
		width = m.width - 2
		if width < 52 {
			width = 52
		}
		if width > 112 {
			width = 112
		}
	}

	pathStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6A83C2")).Bold(true)
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color("#66739A"))
	brand := lipgloss.NewStyle().Foreground(lipgloss.Color("#D7E0FF")).Bold(true)

	topLine := pathStyle.Render("~/" + m.cwd + " " + m.branch)
	subLine := muted.Render("hepctl > ") + brand.Render("hepctl")

	panelStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#5A74B5")).
		Padding(0, 1).
		Width(width)

	titleLine := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#D8E1FF")).
		Bold(true).
		Render("o hepctl (preview) v0.2.0")

	infoPanel := panelStyle.Render(titleLine)

	sessionLines := []string{
		lipgloss.NewStyle().Foreground(lipgloss.Color("#D8E1FF")).Bold(true).Render("context"),
		muted.Render("  |_ working: ~/") + m.cwd,
		muted.Render("  |_ platform: ") + m.platform,
		muted.Render("  |_ cpu arch: ") + m.cpuArch,
		muted.Render("  |_ managers: ") + m.managerState,
		muted.Render("  |_ last cmd: ") + m.lastCommand,
	}
	sessionPanel := panelStyle.Render(strings.Join(sessionLines, "\n"))

	activityPanel := panelStyle.Render(m.renderActivity(width - 4))

	inputStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#5A74B5")).
		Padding(0, 1).
		Width(width)

	inputLine := m.renderInputLine()
	inputPanel := inputStyle.Render(inputLine)

	suggestions := m.installSuggestions()
	hint := muted.Render("try: ") +
		lipgloss.NewStyle().Foreground(lipgloss.Color("#B6C3F2")).Render("install root") +
		muted.Render(" | help | quit")
	if m.versionSelectMode {
		hint = lipgloss.NewStyle().Foreground(lipgloss.Color("#B6C3F2")).Render("↑/↓ or j/k") +
			muted.Render(" to move, ") +
			lipgloss.NewStyle().Foreground(lipgloss.Color("#B6C3F2")).Render("Enter") +
			muted.Render(" to select, ") +
			lipgloss.NewStyle().Foreground(lipgloss.Color("#B6C3F2")).Render("Esc") +
			muted.Render(" to cancel")
	} else if m.passwordMode {
		hint = lipgloss.NewStyle().Foreground(lipgloss.Color("#F2C85B")).Render("🔒 ") +
			muted.Render("Enter to submit, Esc to cancel")
	} else if m.awaitingManager {
		hint = muted.Render("choose manager: ") +
			lipgloss.NewStyle().Foreground(lipgloss.Color("#B6C3F2")).Render("brew") +
			muted.Render(" | ") +
			lipgloss.NewStyle().Foreground(lipgloss.Color("#B6C3F2")).Render("port") +
			muted.Render(" | cancel")
	} else if len(suggestions) > 0 {
		selected := m.suggestIndex
		if selected < 0 || selected >= len(suggestions) {
			selected = 0
		}
		options := make([]string, 0, len(suggestions))
		for idx, suggestion := range suggestions {
			if idx == selected {
				options = append(options, lipgloss.NewStyle().
					Foreground(lipgloss.Color("#0B1020")).
					Background(lipgloss.Color("#B6C3F2")).
					Bold(true).
					Padding(0, 1).
					Render(suggestion))
			} else {
				options = append(options, lipgloss.NewStyle().
					Foreground(lipgloss.Color("#B6C3F2")).
					Render(suggestion))
			}
		}
		hint = muted.Render("autocomplete: ") +
			strings.Join(options, muted.Render(" | ")) +
			muted.Render(" (Tab accept, ↑/↓ or j/k move)")
	}

	statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#8A96BF"))
	if m.statusError {
		statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#F28B9A"))
	}
	statusText := m.status
	if m.running {
		spinner := []string{"-", "\\", "|", "/"}[m.spin%4]
		statusText = spinner + " " + m.status
	}

	view := lipgloss.JoinVertical(
		lipgloss.Left,
		topLine,
		subLine,
		"",
		infoPanel,
		"",
		sessionPanel,
		"",
		activityPanel,
		"",
		inputPanel,
		hint,
		statusStyle.Render(statusText),
	)

	return doc.Render(view)
}

func (m dashboardModel) renderActivity(width int) string {
	// Version selection mode: render the version picker instead of logs.
	if m.versionSelectMode && len(m.availableVersions) > 0 {
		return m.renderVersionPicker(width)
	}

	head := lipgloss.NewStyle().Foreground(lipgloss.Color("#D8E1FF")).Bold(true).Render("activity")
	lines := []string{head}

	maxLines := 10
	if m.height > 34 {
		maxLines = 14
	}
	if len(m.logs) > maxLines {
		m.logs = m.logs[len(m.logs)-maxLines:]
	}

	if len(m.logs) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("#5F6C95")).Render("No activity yet."))
	} else {
		logStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#9EA9CE")).MaxWidth(width)
		okStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("2")).MaxWidth(width)
		errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("1")).MaxWidth(width)
		for _, line := range m.logs {
			switch {
			case strings.HasPrefix(line, "[ok]"):
				lines = append(lines, okStyle.Render(line))
			case strings.HasPrefix(line, "[error]"):
				lines = append(lines, errStyle.Render(line))
			default:
				lines = append(lines, logStyle.Render(line))
			}
		}
	}

	return strings.Join(lines, "\n")
}

func (m dashboardModel) renderVersionPicker(_ int) string {
	head := lipgloss.NewStyle().Foreground(lipgloss.Color("#D8E1FF")).Bold(true).Render("Select ROOT version")
	lines := []string{head, ""}

	normal := lipgloss.NewStyle().Foreground(lipgloss.Color("#9EA9CE"))
	active := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#0B1020")).
		Background(lipgloss.Color("#B6C3F2")).
		Bold(true).
		Padding(0, 1)
	latestBadge := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#0B1020")).
		Background(lipgloss.Color("#7DCEA0")).
		Bold(true).
		Padding(0, 1)
	dateMuted := lipgloss.NewStyle().Foreground(lipgloss.Color("#66739A"))

	for idx, ver := range m.availableVersions {
		label := ver.Version
		dateStr := ""
		if ver.Date != "" {
			dateStr = dateMuted.Render("  " + ver.Date)
		}
		badge := ""
		if ver.IsLatest {
			badge = " " + latestBadge.Render("latest")
		}

		if idx == m.versionCursor {
			lines = append(lines, "  "+active.Render("> "+label)+badge+dateStr)
		} else {
			lines = append(lines, "  "+normal.Render("  "+label)+badge+dateStr)
		}
	}

	return strings.Join(lines, "\n")
}

func (m dashboardModel) renderInputLine() string {
	if m.versionSelectMode {
		prefix := lipgloss.NewStyle().Foreground(lipgloss.Color("#8FA1D6")).Render("> ")
		placeholder := lipgloss.NewStyle().Foreground(lipgloss.Color("#5F6C95")).Render("select a ROOT version above...")
		return prefix + placeholder
	}
	if m.passwordMode {
		return m.renderPasswordLine()
	}
	if m.running {
		prefix := lipgloss.NewStyle().Foreground(lipgloss.Color("#8FA1D6")).Render("> ")
		placeholder := lipgloss.NewStyle().Foreground(lipgloss.Color("#5F6C95")).Render("installation in progress...")
		return prefix + placeholder
	}

	input := make([]rune, len(m.input))
	copy(input, m.input)

	left := string(input[:m.cursor])
	right := string(input[m.cursor:])

	cursor := lipgloss.NewStyle().
		Background(lipgloss.Color("#D8E1FF")).
		Foreground(lipgloss.Color("#0B1020")).
		Render(" ")

	if m.cursor < len(input) {
		cursor = lipgloss.NewStyle().
			Background(lipgloss.Color("#D8E1FF")).
			Foreground(lipgloss.Color("#0B1020")).
			Render(string(input[m.cursor]))
		right = string(input[m.cursor+1:])
	}

	prefix := lipgloss.NewStyle().Foreground(lipgloss.Color("#8FA1D6")).Render("> ")
	if len(input) == 0 {
		placeholder := "install root"
		if m.awaitingManager {
			placeholder = "brew"
		}
		return prefix + cursor + lipgloss.NewStyle().Foreground(lipgloss.Color("#5F6C95")).Render(placeholder)
	}

	return prefix + left + cursor + right
}

func (m dashboardModel) renderPasswordLine() string {
	lockIcon := lipgloss.NewStyle().Foreground(lipgloss.Color("#F2C85B")).Render("🔒 ")
	label := lipgloss.NewStyle().Foreground(lipgloss.Color("#D8E1FF")).Bold(true).Render("sudo password: ")

	masked := strings.Repeat("•", len(m.passwordInput))
	maskedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#B6C3F2")).Render(masked)

	cursor := lipgloss.NewStyle().
		Background(lipgloss.Color("#D8E1FF")).
		Foreground(lipgloss.Color("#0B1020")).
		Render(" ")

	return lockIcon + label + maskedStyle + cursor
}

// trySudoAuth validates the password using "sudo -S -v" (reads password from stdin).
// Returns a tea.Cmd that runs asynchronously and produces a sudoResultMsg.
func trySudoAuth(password string) tea.Cmd {
	return func() tea.Msg {
		cmd := exec.Command("sudo", "-S", "-v")
		cmd.Stdin = strings.NewReader(password + "\n")
		err := cmd.Run()
		// Zero out the password string's backing bytes (best-effort).
		pw := []byte(password)
		for i := range pw {
			pw[i] = 0
		}
		return sudoResultMsg{err: err}
	}
}

func insertAt(base []rune, pos int, chunk []rune) []rune {
	if pos < 0 {
		pos = 0
	}
	if pos > len(base) {
		pos = len(base)
	}

	out := make([]rune, 0, len(base)+len(chunk))
	out = append(out, base[:pos]...)
	out = append(out, chunk...)
	out = append(out, base[pos:]...)
	return out
}

func normalizeCommand(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(s))), " ")
}

func (m dashboardModel) installSuggestions() []string {
	if m.running || m.awaitingManager {
		return nil
	}

	raw := strings.ToLower(string(m.input))
	trimmed := strings.TrimLeft(raw, " ")
	if !strings.HasPrefix(trimmed, "install") {
		return nil
	}

	rest := strings.TrimPrefix(trimmed, "install")
	if rest == trimmed {
		return nil
	}
	if len(rest) > 0 && rest[0] != ' ' {
		return nil
	}

	fragment := strings.TrimLeft(rest, " ")
	if fragment == "" {
		return nil
	}
	if !containsLetter(fragment) {
		return nil
	}
	if strings.Contains(fragment, " ") {
		return nil
	}

	matches := make([]string, 0, len(installablePackages))
	for _, pkg := range installablePackages {
		if fragment == "" || strings.HasPrefix(pkg, fragment) {
			matches = append(matches, pkg)
		}
	}

	if len(matches) == 1 && normalizeCommand(string(m.input)) == "install "+matches[0] {
		return nil
	}
	return matches
}

func containsLetter(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

func currentDirName() string {
	wd, err := os.Getwd()
	if err != nil {
		return "workspace"
	}
	base := filepath.Base(wd)
	if base == "." || base == string(filepath.Separator) {
		return "workspace"
	}
	return base
}

func detectGitBranch() string {
	head, err := os.ReadFile(".git/HEAD")
	if err != nil {
		return "no-branch"
	}

	line := strings.TrimSpace(string(head))
	if strings.HasPrefix(line, "ref: ") {
		parts := strings.Split(line, "/")
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
	}

	if len(line) > 8 {
		return line[:8]
	}
	if line == "" {
		return "detached"
	}
	return line
}

func platformLabel() string {
	switch runtime.GOOS {
	case "darwin":
		return "macOS"
	case "linux":
		distro := platform.DistroName()
		return distro + " (Linux)"
	default:
		return runtime.GOOS
	}
}

// needsSudoAuth reports whether sudo credentials need to be (re-)authenticated.
// It runs "sudo -n true" which succeeds silently if credentials are cached.
func needsSudoAuth() bool {
	return exec.Command("sudo", "-n", "true").Run() != nil
}

// needsVersionSelection reports whether the current Linux distro requires
// manual ROOT version selection (i.e. ROOT is not in the distro's repos).
func needsVersionSelection() bool {
	for _, d := range install.DistrosNeedingVersionSelect() {
		if platform.IsDistro(d) {
			return true
		}
	}
	return false
}

// fetchVersionsCmd returns a tea.Cmd that fetches ROOT versions in the background.
func fetchVersionsCmd() tea.Cmd {
	return func() tea.Msg {
		versions, err := install.FetchROOTVersions()
		return versionsFetchedMsg{versions: versions, err: err}
	}
}

func detectManagerState() string {
	switch runtime.GOOS {
	case "linux":
		mgr := platform.PackageManager()
		if mgr == "unknown" {
			return "none detected"
		}
		return mgr
	case "darwin":
		probe := install.NewRootInstaller(install.DefaultRunner{}, nil, io.Discard)
		detected := probe.DetectMacManagers()
		switch {
		case detected.BrewExists && detected.PortExists:
			return "brew + port"
		case detected.BrewExists:
			return "brew"
		case detected.PortExists:
			return "port"
		default:
			return "none (will install brew)"
		}
	default:
		return "unsupported"
	}
}

type streamingRunner struct {
	emit func(line string)
}

func (r *streamingRunner) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

func (r *streamingRunner) Run(ctx context.Context, name string, args ...string) error {
	r.emit("$ " + name + " " + strings.Join(args, " "))

	cmd := exec.CommandContext(ctx, name, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		streamLines(stdout, r.emit)
	}()
	go func() {
		defer wg.Done()
		streamLines(stderr, r.emit)
	}()

	waitErr := cmd.Wait()
	wg.Wait()
	return waitErr
}

func streamLines(in io.Reader, emit func(line string)) {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		emit(scanner.Text())
	}
}

type eventWriter struct {
	mu   sync.Mutex
	buf  bytes.Buffer
	emit func(string)
}

func newEventWriter(emit func(string)) *eventWriter {
	return &eventWriter{emit: emit}
}

func (w *eventWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.buf.Write(p)
	for {
		data := w.buf.Bytes()
		idx := bytes.IndexByte(data, '\n')
		if idx < 0 {
			break
		}
		line := strings.TrimSpace(string(data[:idx]))
		w.buf.Next(idx + 1)
		if line != "" {
			w.emit(line)
		}
	}
	return len(p), nil
}

func (w *eventWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()

	remaining := strings.TrimSpace(w.buf.String())
	if remaining != "" {
		w.emit(remaining)
	}
	w.buf.Reset()
}
