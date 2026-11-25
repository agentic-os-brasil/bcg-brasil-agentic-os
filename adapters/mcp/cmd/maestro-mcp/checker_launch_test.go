package main

import (
	"os"
	"testing"
)

func TestInstalledCheckServeDispatchRefusesUnboundStartup(t *testing.T) {
	previous := os.Args
	defer func() { os.Args = previous }()
	os.Args = []string{"maestro-mcp", "installed-check-serve"}
	if err := run(); err == nil {
		t.Fatal("checker child dispatch accepted unbound startup")
	}
}
