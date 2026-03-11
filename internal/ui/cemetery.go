package ui

import (
	"fmt"
	"strings"
	"time"

	"atlas.grave/internal/model"
	"atlas.grave/internal/system"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	amber  = lipgloss.Color("#FFB642")
	rusty  = lipgloss.Color("#5E4737")
	ncrRed = lipgloss.Color("#FF0000")
	onyx   = lipgloss.Color("#050505")

	screenStyle = lipgloss.NewStyle().
			Padding(2, 4).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(amber).
			Background(onyx)

	headerStyle = lipgloss.NewStyle().
			Foreground(onyx).
			Background(amber).
			Padding(0, 1).
			Bold(true)

	selectedStyle = lipgloss.NewStyle().
			Foreground(onyx).
			Background(amber).
			Bold(true)

	textStyle   = lipgloss.NewStyle().Foreground(amber)
	dimStyle    = lipgloss.NewStyle().Foreground(rusty)
	restlessStyle = lipgloss.NewStyle().Foreground(ncrRed).Bold(true)
)

type state int
const (
	stateBrowsing state = iota
	stateConfirming
	stateSearching
)

type Model struct {
	reaper        *system.Reaper
	souls         []model.Soul
	filtered      []model.Soul
	cursor        int
	width         int
	height        int
	state         state
	reclaimed     uint64
	input         textinput.Model
}

func NewModel(r *system.Reaper) Model {
	ti := textinput.New()
	ti.Placeholder = "TYPE TO SEARCH..."
	ti.Prompt = " > "
	ti.TextStyle = textStyle

	return Model{
		reaper: r,
		souls:  []model.Soul{},
		input:  ti,
	}
}

type scanMsg []model.Soul
type tickMsg time.Time

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.scanCmd(), tick())
}

func (m Model) scanCmd() tea.Cmd {
	return func() tea.Msg {
		souls, _ := m.reaper.Scan()
		return scanMsg(souls)
	}
}

func tick() tea.Cmd {
	return tea.Tick(time.Second*2, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.state == stateSearching {
			switch msg.String() {
			case "enter", "esc":
				m.state = stateBrowsing
				return m, nil
			}
			m.input, cmd = m.input.Update(msg)
			m.filterSouls()
			return m, cmd
		}

		if m.state == stateConfirming {
			switch msg.String() {
			case "y", "Y", "enter":
				if len(m.filtered) > 0 && m.cursor < len(m.filtered) {
					soul := m.filtered[m.cursor]
					m.reclaimed += soul.Memory
					m.reaper.Bury(soul.PID)
				}
				m.state = stateBrowsing
				return m, m.scanCmd()
			case "n", "N", "esc":
				m.state = stateBrowsing
				return m, nil
			}
		}

		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 { m.cursor-- }
		case "down", "j":
			if m.cursor < len(m.filtered)-1 { m.cursor++ }
		case "pgup":
			m.cursor -= 10
			if m.cursor < 0 { m.cursor = 0 }
		case "pgdown":
			m.cursor += 10
			if m.cursor >= len(m.filtered) { m.cursor = len(m.filtered)-1 }
		case "/", "s":
			m.state = stateSearching
			m.input.Focus()
			return m, nil
		case "enter", "b":
			if len(m.filtered) > 0 {
				m.state = stateConfirming
			}
		}

	case scanMsg:
		m.souls = msg
		m.filterSouls()
		if m.cursor >= len(m.filtered) && len(m.filtered) > 0 {
			m.cursor = len(m.filtered)-1
		}
	case tickMsg:
		if m.state == stateBrowsing {
			return m, tea.Batch(m.scanCmd(), tick())
		}
		return m, tick()
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	}
	return m, nil
}

func (m *Model) filterSouls() {
	query := strings.ToLower(m.input.Value())
	if query == "" {
		m.filtered = m.souls
		return
	}

	var filtered []model.Soul
	for _, s := range m.souls {
		if strings.Contains(strings.ToLower(s.Name), query) || strings.Contains(fmt.Sprintf("%d", s.PID), query) {
			filtered = append(filtered, s)
		}
	}
	m.filtered = filtered
}

func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit { return fmt.Sprintf("%d B", b) }
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func (m Model) View() string {
	if m.width == 0 || m.height == 0 { return "" }
	var sb strings.Builder

	header := headerStyle.Render(" ATLAS REAPER 3000 ") + " " + textStyle.Render(fmt.Sprintf("RECLAIMED: %s", formatBytes(m.reclaimed)))
	sb.WriteString(header + "\n\n")

	if m.state == stateSearching {
		sb.WriteString(headerStyle.Render(" SEARCH DATABASE ") + "\n")
		sb.WriteString(m.input.View() + "\n\n")
	}

	if len(m.filtered) == 0 {
		sb.WriteString(dimStyle.Render("NO RESTLESS SOULS FOUND.") + "\n")
	} else {
		sb.WriteString(dimStyle.Render(fmt.Sprintf("%-8s %-30s %-10s %-15s", "SOUL ID", "NAME", "SIN", "BURDEN")) + "\n")
		
		start := 0
		if m.cursor > 15 { start = m.cursor - 15 }
		end := start + 25
		if end > len(m.filtered) { end = len(m.filtered) }

		for i := start; i < end; i++ {
			s := m.filtered[i]
			cpuStr := fmt.Sprintf("%.1f%%", s.CPU)
			line := fmt.Sprintf("%-8d %-30.30s %-10s %-15s", s.PID, s.Name, cpuStr, formatBytes(s.Memory))
			
			style := textStyle
			if s.CPU > 50 || s.Memory > 1024*1024*500 {
				style = restlessStyle
			}

			if i == m.cursor {
				sb.WriteString(selectedStyle.Render("> "+line) + "\n")
			} else {
				sb.WriteString("  " + style.Render(line) + "\n")
			}
		}
	}

	if m.state == stateConfirming && len(m.filtered) > 0 {
		target := m.filtered[m.cursor]
		sb.WriteString("\n" + restlessStyle.Render(fmt.Sprintf("BURY SOUL %s (PID %d)? (Y/n)", target.Name, target.PID)))
	} else if m.state != stateSearching {
		sb.WriteString("\n" + dimStyle.Render("J/K: NAVIGATE • PGUP/PGDN: SPEED • /: SEARCH • ENTER: BURY"))
	}

	return screenStyle.Render(sb.String())
}
