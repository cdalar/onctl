package ui

import (
	"fmt"
	"os"
	"sync"
	"time"

	"golang.org/x/term"
)

// spinnerFrames and spinnerInterval mirror the "Dot" spinner previously
// rendered via charmbracelet/bubbles, kept for visual parity.
var spinnerFrames = []string{"⣾ ", "⣽ ", "⣻ ", "⢿ ", "⡿ ", "⣟ ", "⣯ ", "⣷ "}

const spinnerInterval = 100 * time.Millisecond

// spinnerColor is the SGR sequence used to color the spinner glyph (ANSI 256
// color 205, matching the previous lipgloss style), reset afterwards.
const (
	spinnerColorOn  = "\033[38;5;205m"
	spinnerColorOff = "\033[0m"
)

// Spinner is a minimal terminal spinner that writes plain ANSI escape
// sequences directly to stderr. Unlike the bubbletea-based implementation it
// replaces, it never probes terminal capabilities (e.g. background-color
// queries), so it can't block on an unresponsive pty.
type Spinner struct {
	mu        sync.Mutex
	Suffix    string // Public field to match briandowns/spinner API
	suffix    string // suffix currently rendered by the running loop
	isRunning bool
	stopCh    chan struct{}
	done      chan struct{} // closed when the render loop exits
}

// New creates a new spinner with a dots style
func New() *Spinner {
	return &Spinner{}
}

// Start begins the spinner animation
func (s *Spinner) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isRunning {
		return
	}

	s.suffix = s.Suffix
	s.isRunning = true

	// Check if we are in a terminal
	if !term.IsTerminal(int(os.Stderr.Fd())) {
		s.stopCh = nil
		s.done = make(chan struct{})
		close(s.done)
		return
	}

	stopCh := make(chan struct{})
	done := make(chan struct{})
	s.stopCh = stopCh
	s.done = done

	go s.run(stopCh, done)
}

func (s *Spinner) run(stopCh, done chan struct{}) {
	defer close(done)

	ticker := time.NewTicker(spinnerInterval)
	defer ticker.Stop()

	fmt.Fprint(os.Stderr, "\033[?25l") // hide cursor
	defer fmt.Fprint(os.Stderr, "\033[?25h")

	frame := 0
	for {
		s.mu.Lock()
		suffix := s.suffix
		s.mu.Unlock()

		fmt.Fprintf(os.Stderr, "\r\033[K%s%s%s%s", spinnerColorOn, spinnerFrames[frame%len(spinnerFrames)], spinnerColorOff, suffix)

		select {
		case <-stopCh:
			fmt.Fprint(os.Stderr, "\r\033[K")
			return
		case <-ticker.C:
			frame++
		}
	}
}

// Stop stops the spinner animation
func (s *Spinner) Stop() {
	s.mu.Lock()
	if !s.isRunning {
		s.mu.Unlock()
		return
	}

	s.isRunning = false
	stopCh := s.stopCh
	done := s.done
	s.mu.Unlock()

	if stopCh != nil {
		close(stopCh)
	}
	if done != nil {
		<-done
	}
}

// Restart stops and starts the spinner
func (s *Spinner) Restart() {
	s.Stop()
	s.Start()
}

// SetSuffix sets the text that appears after the spinner
func (s *Spinner) SetSuffix(suffix string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Suffix = suffix
	s.suffix = suffix
}

// Active returns whether the spinner is currently running
func (s *Spinner) Active() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.isRunning
}
