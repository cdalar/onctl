package ui

import (
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	s := New()
	if s == nil {
		t.Fatal("New() returned nil")
	}
	if s.isRunning {
		t.Error("New spinner should not be running")
	}
	if s.Suffix != "" {
		t.Errorf("New spinner should have empty suffix, got %q", s.Suffix)
	}
}

func TestSpinnerStartStop(t *testing.T) {
	s := New()

	// Test Start
	s.Start()
	// Give it a moment to start
	time.Sleep(20 * time.Millisecond)

	if !s.Active() {
		t.Error("Spinner should be active after Start()")
	}

	// Test Stop
	s.Stop()
	// Give it a moment to stop
	time.Sleep(20 * time.Millisecond)

	if s.Active() {
		t.Error("Spinner should not be active after Stop()")
	}
}

func TestSpinnerRestart(t *testing.T) {
	s := New()

	// Start spinner
	s.Start()
	time.Sleep(20 * time.Millisecond)

	if !s.Active() {
		t.Error("Spinner should be active after Start()")
	}

	// Restart spinner
	s.Restart()
	time.Sleep(20 * time.Millisecond)

	if !s.Active() {
		t.Error("Spinner should be active after Restart()")
	}

	// Clean up
	s.Stop()
	time.Sleep(20 * time.Millisecond)
}

func TestSpinnerSuffix(t *testing.T) {
	s := New()

	// Test setting suffix before start
	testSuffix := " Testing spinner..."
	s.Suffix = testSuffix

	if s.Suffix != testSuffix {
		t.Errorf("Expected suffix %q, got %q", testSuffix, s.Suffix)
	}

	// Test setting suffix while running
	s.Start()
	time.Sleep(20 * time.Millisecond)

	newSuffix := " New suffix"
	s.SetSuffix(newSuffix)

	if s.Suffix != newSuffix {
		t.Errorf("Expected suffix %q, got %q", newSuffix, s.Suffix)
	}

	if s.model.suffix != newSuffix {
		t.Errorf("Expected model suffix %q, got %q", newSuffix, s.model.suffix)
	}

	s.Stop()
	time.Sleep(20 * time.Millisecond)
}

func TestSpinnerActive(t *testing.T) {
	s := New()

	// Should not be active initially
	if s.Active() {
		t.Error("New spinner should not be active")
	}

	// Should be active after start
	s.Start()
	time.Sleep(20 * time.Millisecond)

	if !s.Active() {
		t.Error("Spinner should be active after Start()")
	}

	// Should not be active after stop
	s.Stop()
	time.Sleep(20 * time.Millisecond)

	if s.Active() {
		t.Error("Spinner should not be active after Stop()")
	}
}

func TestSpinnerMultipleStarts(t *testing.T) {
	s := New()

	// Start multiple times should be safe
	s.Start()
	time.Sleep(20 * time.Millisecond)
	s.Start() // Second start should be no-op
	time.Sleep(20 * time.Millisecond)

	if !s.Active() {
		t.Error("Spinner should still be active")
	}

	s.Stop()
	time.Sleep(20 * time.Millisecond)
}

func TestSpinnerMultipleStops(t *testing.T) {
	s := New()

	s.Start()
	time.Sleep(20 * time.Millisecond)

	// Stop multiple times should be safe
	s.Stop()
	time.Sleep(20 * time.Millisecond)
	s.Stop() // Second stop should be no-op
	time.Sleep(20 * time.Millisecond)

	if s.Active() {
		t.Error("Spinner should not be active")
	}
}

func TestSpinnerConcurrency(t *testing.T) {
	s := New()

	// Test concurrent access
	done := make(chan bool)

	// Goroutine 1: Start/Stop
	go func() {
		s.Start()
		time.Sleep(10 * time.Millisecond)
		s.Stop()
		done <- true
	}()

	// Goroutine 2: Check Active
	go func() {
		for i := 0; i < 5; i++ {
			_ = s.Active()
			time.Sleep(5 * time.Millisecond)
		}
		done <- true
	}()

	// Goroutine 3: Set Suffix
	go func() {
		for i := 0; i < 5; i++ {
			s.SetSuffix(" Testing concurrent access")
			time.Sleep(5 * time.Millisecond)
		}
		done <- true
	}()

	// Wait for all goroutines
	for i := 0; i < 3; i++ {
		<-done
	}

	// Clean up
	s.Stop()
	time.Sleep(20 * time.Millisecond)
}
