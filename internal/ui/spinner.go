package ui

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Spinner wraps the Bubble Tea spinner to provide a simple API similar to briandowns/spinner
type Spinner struct {
	model      model
	program    *tea.Program
	mu         sync.Mutex
	Suffix     string // Public field to match briandowns/spinner API
	isRunning  bool
	hideOutput bool
}

type model struct {
	spinner spinner.Model
	suffix  string
}

func (m model) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case tea.KeyMsg:
		return m, tea.Quit
	default:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
}

func (m model) View() string {
	if m.suffix == "" {
		return m.spinner.View()
	}
	return lipgloss.JoinHorizontal(lipgloss.Left, m.spinner.View(), m.suffix)
}

// New creates a new spinner with a dots style
func New() *Spinner {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	return &Spinner{
		model: model{
			spinner: s,
		},
		hideOutput: false,
	}
}

// Start begins the spinner animation
func (s *Spinner) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isRunning {
		return
	}

	// Sync the suffix
	s.model.suffix = s.Suffix

	s.program = tea.NewProgram(s.model, tea.WithOutput(os.Stderr))
	s.isRunning = true

	go func() {
		if _, err := s.program.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error running spinner: %v\n", err)
		}
	}()

	// Give the program a moment to start
	time.Sleep(10 * time.Millisecond)
}

// Stop stops the spinner animation
func (s *Spinner) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.isRunning {
		return
	}

	if s.program != nil {
		s.program.Quit()
		// Give the program a moment to clean up
		time.Sleep(10 * time.Millisecond)
	}

	s.isRunning = false
}

// Restart stops and starts the spinner
func (s *Spinner) Restart() {
	s.Stop()
	// Sync the suffix before restarting
	s.mu.Lock()
	s.model.suffix = s.Suffix
	s.mu.Unlock()
	s.Start()
}

// SetSuffix sets the text that appears after the spinner
func (s *Spinner) SetSuffix(suffix string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Suffix = suffix
	s.model.suffix = suffix

	if s.isRunning && s.program != nil {
		s.program.Send(s.model)
	}
}

// Active returns whether the spinner is currently running
func (s *Spinner) Active() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.isRunning
}
