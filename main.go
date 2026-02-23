package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/cdalar/onctl/cmd"

	"github.com/hashicorp/logutils"
	"golang.org/x/term"
)

func main() {
	// Save terminal state before any spinner starts so we can fully restore it
	// (cursor visibility, echo mode, cooked mode) on abnormal exit.
	var savedState *term.State
	if term.IsTerminal(int(os.Stdin.Fd())) {
		if state, err := term.GetState(int(os.Stdin.Fd())); err == nil {
			savedState = state
		}
	}

	// On SIGINT/SIGTERM restore the terminal before exiting.
	// Without this, pressing Ctrl+C while a spinner is running leaves the
	// terminal in raw mode (no echo) with the cursor hidden.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		if savedState != nil {
			_ = term.Restore(int(os.Stdin.Fd()), savedState)
		}
		fmt.Fprint(os.Stderr, "\033[?25h") // show cursor
		os.Exit(1)
	}()

	filter := &logutils.LevelFilter{
		Levels:   []logutils.LogLevel{"DEBUG", "WARN", "ERROR"},
		MinLevel: logutils.LogLevel("WARN"),
		Writer:   os.Stderr,
	}
	if os.Getenv("ONCTL_LOG") != "" {
		filter.MinLevel = logutils.LogLevel(os.Getenv("ONCTL_LOG"))
		log.SetFlags(log.Lshortfile)
	}
	log.SetOutput(filter)
	err := cmd.Execute()
	if err != nil {
		log.Println(err)
	}
}
