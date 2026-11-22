package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestMaintenanceCLIRejectsExpiredOperationBeforeAdmission(t *testing.T) {
	prior := os.Args
	defer func() { os.Args = prior }()
	os.Args = []string{"maestro-mcp", "maintenance-status", "--maintenance-deadline", time.Now().Add(-time.Second).Format(time.RFC3339Nano)}
	if err := run(); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expired maintenance command did not refuse before admission: %v", err)
	}
}

func TestMaintenanceCLIEmitsOnlyClosedDiagnostic(t *testing.T) {
	child := exec.Command(os.Args[0], "-test.run=^TestMaintenanceCLIClosedDiagnosticHelper$")
	child.Env = append(os.Environ(), "MAESTRO_TEST_MAINTENANCE_DIAGNOSTIC=1", "MAESTRO_TEST_MAINTENANCE_PROGRESS=1")
	var stdout, stderr bytes.Buffer
	child.Stdout, child.Stderr = &stdout, &stderr
	if err := child.Run(); err == nil {
		t.Fatal("invalid admitted root unexpectedly succeeded")
	}
	if stdout.Len() != 0 || stderr.Len() == 0 || stderr.Len() > 4096 {
		t.Fatalf("unexpected output sizes: stdout=%d stderr=%d", stdout.Len(), stderr.Len())
	}
	if strings.Contains(stderr.String(), "private-error-sentinel") {
		t.Fatal("private input escaped in maintenance error")
	}
	terminal := false
	for _, line := range strings.Split(strings.TrimSpace(stderr.String()), "\n") {
		var metadata struct {
			SchemaVersion int    `json:"schema_version"`
			Phase         string `json:"phase"`
			Code          string `json:"code"`
		}
		if err := json.Unmarshal([]byte(line), &metadata); err != nil {
			t.Fatalf("maintenance error was not closed metadata: %q", line)
		}
		if metadata.SchemaVersion != 1 || metadata.Phase != "admission" {
			t.Fatalf("unexpected maintenance diagnostic: %+v", metadata)
		}
		if metadata.Code == "maintenance_rejected" {
			terminal = true
		} else if metadata.Code != "in_progress" {
			t.Fatalf("unexpected error category: %q", metadata.Code)
		}
	}
	if !terminal {
		t.Fatal("no terminal closed failure code")
	}
}

func TestMaintenanceCLIClosedDiagnosticHelper(t *testing.T) {
	if os.Getenv("MAESTRO_TEST_MAINTENANCE_DIAGNOSTIC") != "1" {
		return
	}
	os.Args = []string{"maestro-mcp", "maintenance-status",
		"--product-root", "/synthetic/private-error-sentinel/product",
		"--user-home", "/synthetic/private-error-sentinel/owner",
		"--workspace", "/synthetic/private-error-sentinel/workspace"}
	if os.Getenv("MAESTRO_TEST_MAINTENANCE_PROGRESS") == "1" {
		os.Args = append(os.Args, "--maintenance-progress")
	}
	main()
}
