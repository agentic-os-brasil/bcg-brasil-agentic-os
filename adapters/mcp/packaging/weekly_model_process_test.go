package packaging

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

// This binary is a synthetic protocol fixture, never a model/native tuple.
func TestWeeklyCLIProcessHelper(t *testing.T) {
	mode := os.Getenv("MAESTRO_WEEKLY_SYNTHETIC")
	if mode == "" {
		return
	}
	if mode == "drain" {
		_, _ = io.WriteString(os.Stdout, strings.Repeat("x", 256))
		_ = os.WriteFile(os.Getenv("MAESTRO_WEEKLY_DRAIN_READY"), []byte("ready"), 0600)
		time.Sleep(2 * time.Second)
		os.Exit(0)
	}
	if mode == "stall-input" {
		time.Sleep(2 * time.Second)
		os.Exit(0)
	}
	body, _ := io.ReadAll(io.LimitReader(os.Stdin, 32769))
	switch mode {
	case "echo":
		_, _ = os.Stdout.Write(body)
	case "overflow":
		_, _ = io.WriteString(os.Stdout, strings.Repeat("x", 8192))
	case "stderr":
		_, _ = io.WriteString(os.Stderr, strings.Repeat("x", 8192))
	case "cancel":
		time.Sleep(10 * time.Second)
	case "fail":
		os.Exit(3)
	}
	os.Exit(0)
}

func TestWeeklyCLISyntheticProcessBoundsAndCancellation(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"echo", "overflow", "stderr", "cancel", "fail"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			if mode == "cancel" {
				cancel()
				ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
			}
			defer cancel()
			started := time.Now()
			out, err := runWeeklyModelProcess(ctx, weeklyModelProcess{Path: executable, Args: []string{"-test.run=^TestWeeklyCLIProcessHelper$"}, Directory: t.TempDir(), Environment: []string{"MAESTRO_WEEKLY_SYNTHETIC=" + mode}, Input: []byte("synthetic-private-input"), MaxOutput: 1024, MaxStderr: 1024})
			if mode == "echo" {
				if err != nil || string(out) != "synthetic-private-input" {
					t.Fatal("synthetic stdin/closed output failed", err)
				}
			} else if err == nil || len(out) != 0 {
				t.Fatal("failed/overflow/cancelled transport returned usable private output")
			}
			if time.Since(started) > 4*time.Second {
				t.Fatal("synthetic process closure exceeded bound")
			}
		})
	}
}
