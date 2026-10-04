package cmd

import (
	"bytes"
	"io"
	"net"
	"strings"
	"testing"
)

// relayStdio must keep reading what the VM sends after our side's input
// ends -- ssh closes its write half long before the last bytes arrive.
func TestRelayStdio(t *testing.T) {
	ours, vm := net.Pipe()
	go func() {
		got, _ := io.ReadAll(io.LimitReader(vm, 5))
		_, _ = vm.Write([]byte("echo: " + string(got)))
		_ = vm.Close()
	}()
	var out bytes.Buffer
	if err := relayStdio(ours, strings.NewReader("hello"), &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != "echo: hello" {
		t.Fatalf("got %q", out.String())
	}
}
