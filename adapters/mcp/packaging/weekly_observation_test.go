package packaging

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWeeklyRuntimeObservationRefusesScriptBeforeExecution(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "executed")
	path := filepath.Join(root, "claude")
	if err := os.WriteFile(path, []byte("#!/bin/sh\ntouch '"+marker+"'\nprintf '2.1.289\\n'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	env, err := WeeklyModelEnvironment([]string{"HOME=" + root, "CLAUDE_CONFIG_DIR=" + root, "PATH=/usr/bin:/bin"}, 2048)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := ObserveWeeklyRuntime(ctx, path, env, root); err == nil {
		t.Fatal("script cannot declare itself a supported signed native runtime")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("unverified executable ran: %v", err)
	}
}
