package ui

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var errSelectionCanceled = errors.New("selection canceled")

// Prompter handles interactive selections.
type Prompter interface {
	Choose(label string, options []string) (int, error)
}

// StandardPrompter uses a rich TUI selection UI when running in a TTY and
// falls back to numeric prompts for non-interactive environments.
type StandardPrompter struct {
	In  *os.File
	Out *os.File
}

func NewPrompter(in *os.File, out *os.File) Prompter {
	return StandardPrompter{In: in, Out: out}
}

func (p StandardPrompter) Choose(label string, options []string) (int, error) {
	if len(options) == 0 {
		return -1, fmt.Errorf("no options provided")
	}

	if canUseTUI(p.In, p.Out) {
		choice, err := runChooser(label, options)
		if err == nil {
			return choice, nil
		}
		if errors.Is(err, errSelectionCanceled) {
			return -1, err
		}
	}

	return chooseNumeric(p.In, p.Out, label, options)
}

func chooseNumeric(in io.Reader, out io.Writer, label string, options []string) (int, error) {
	reader := bufio.NewReader(in)

	for {
		fmt.Fprintln(out, label)
		for idx, option := range options {
			fmt.Fprintf(out, "  %d) %s\n", idx+1, option)
		}
		fmt.Fprintf(out, "Enter choice [1-%d]: ", len(options))

		line, err := reader.ReadString('\n')
		if err != nil {
			return -1, err
		}

		line = strings.TrimSpace(line)
		selected, err := strconv.Atoi(line)
		if err != nil || selected < 1 || selected > len(options) {
			fmt.Fprintln(out, "Invalid choice, try again.")
			continue
		}

		return selected - 1, nil
	}
}

func canUseTUI(in *os.File, out *os.File) bool {
	if in == nil || out == nil {
		return false
	}

	if term := strings.TrimSpace(os.Getenv("TERM")); term == "" || term == "dumb" {
		return false
	}

	inInfo, err := in.Stat()
	if err != nil || (inInfo.Mode()&os.ModeCharDevice) == 0 {
		return false
	}

	outInfo, err := out.Stat()
	if err != nil || (outInfo.Mode()&os.ModeCharDevice) == 0 {
		return false
	}

	return true
}

func runChooser(label string, options []string) (int, error) {
	initial := chooserModel{
		label:    label,
		options:  options,
		cursor:   0,
		selected: -1,
	}

	finalModel, err := tea.NewProgram(initial, tea.WithAltScreen()).Run()
	if err != nil {
		return -1, err
	}

	m, ok := finalModel.(chooserModel)
	if !ok {
		return -1, errors.New("unexpected chooser model type")
	}
	if m.selected < 0 {
		return -1, errSelectionCanceled
	}

	return m.selected, nil
}

type chooserModel struct {
	label    string
	options  []string
	cursor   int
	selected int
	width    int
}

func (m chooserModel) Init() tea.Cmd {
	return nil
}

func (m chooserModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch typed := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = typed.Width
	case tea.KeyMsg:
		switch typed.String() {
		case "up", "k":
			m.cursor--
			if m.cursor < 0 {
				m.cursor = len(m.options) - 1
			}
		case "down", "j":
			m.cursor++
			if m.cursor >= len(m.options) {
				m.cursor = 0
			}
		case "enter":
			m.selected = m.cursor
			return m, tea.Quit
		case "q", "esc", "ctrl+c":
			m.selected = -1
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m chooserModel) View() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	promptStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
	choiceStyle := lipgloss.NewStyle().PaddingLeft(1)
	activeStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("230")).
		Background(lipgloss.Color("31")).
		Bold(true).
		Padding(0, 1)
	hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	boxStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("63")).
		Padding(1, 2)

	lines := []string{
		titleStyle.Render("hepctl"),
		promptStyle.Render(m.label),
		"",
	}

	for idx, option := range m.options {
		if idx == m.cursor {
			lines = append(lines, "  "+activeStyle.Render(">"+option))
		} else {
			lines = append(lines, "  "+choiceStyle.Render(" "+option))
		}
	}

	lines = append(lines, "", hintStyle.Render("j/k or arrows to move, enter to select, q to cancel"))
	panel := boxStyle.Render(strings.Join(lines, "\n"))

	if m.width > 0 {
		return lipgloss.PlaceHorizontal(m.width, lipgloss.Center, panel)
	}
	return panel
}
