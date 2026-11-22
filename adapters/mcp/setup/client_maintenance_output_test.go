package setup

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"time"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"io"
	"os"
	"os/exec"
	"testing"
)

func TestMaintenanceCommandOutputDrainsCopyAndSubprocess(t *testing.T) {
	input := bytes.Repeat([]byte("a"), (1<<20)+33)
	source := bytes.NewReader(input)
	collected := maintenanceCommandOutput{limit: 8192}
	count, err := io.Copy(&collected, source)
	if err != nil || count != int64(len(input)) || source.Len() != 0 || !collected.truncated || len(collected.Bytes()) != 8192 {
		t.Fatalf("copy did not preserve/drain the bounded prefix: count=%d left=%d retained=%d overflow=%t err=%v", count, source.Len(), len(collected.Bytes()), collected.truncated, err)
	}
	if n, err := collected.Write(nil); err != nil || n != 0 || !collected.truncated {
		t.Fatal("empty write cleared prior overflow")
	}
	child := exec.Command(os.Args[0], "-test.run=^TestMaintenanceCommandOutputHelper$")
	child.Env = append(os.Environ(), "MAESTRO_TEST_MAINTENANCE_OUTPUT=1")
	stdout, stderr := maintenanceCommandOutput{limit: 8192}, maintenanceCommandOutput{limit: 4096}
	child.Stdout, child.Stderr = &stdout, &stderr
	if err := child.Run(); err != nil {
		t.Fatalf("actual child pipes were not drained: %v", err)
	}
	if !stdout.truncated || len(stdout.Bytes()) != 8192 || !bytes.Equal(stdout.Bytes(), bytes.Repeat([]byte("a"), 8192)) {
		t.Fatal("actual child stdout bypassed the bound")
	}
	if !stderr.truncated || len(stderr.Bytes()) != 4096 || !bytes.Equal(stderr.Bytes(), bytes.Repeat([]byte("b"), 4096)) {
		t.Fatal("actual child stderr bypassed the bound")
	}
}

func TestMaintenanceCommandOutputHelper(t *testing.T) {
	mode := os.Getenv("MAESTRO_TEST_MAINTENANCE_OUTPUT")
	if mode != "1" && mode != "interrupt_overflow" {
		return
	}
	if _, err := os.Stdout.Write(bytes.Repeat([]byte("a"), (1<<20)+33)); err != nil {
		os.Exit(2)
	}
	if mode == "interrupt_overflow" {
		if _, err := os.Stderr.Write([]byte("{\"schema_version\":1,\"phase\":\"execution\",\"code\":\"in_progress\"}\n")); err != nil {
			os.Exit(5)
		}
	}
	if _, err := os.Stderr.Write(bytes.Repeat([]byte("b"), (1<<20)+33)); err != nil {
		os.Exit(3)
	}
	if mode == "interrupt_overflow" {
		if err := os.WriteFile(os.Getenv("MAESTRO_TEST_MAINTENANCE_INTERRUPT_READY"), []byte("ready"), 0600); err != nil {
			os.Exit(6)
		}
		time.Sleep(time.Minute)
		os.Exit(7)
	}
	os.Exit(0)
}

func TestMaintenanceOriginalChildInterruptionWinsOverOutputOverflow(t *testing.T) {
	readyPath := filepath.Join(t.TempDir(), "overflow-drained")
	supervision, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	child := exec.CommandContext(supervision, os.Args[0], "-test.run=^TestMaintenanceCommandOutputHelper$")
	child.Env = append(os.Environ(), "MAESTRO_TEST_MAINTENANCE_OUTPUT=interrupt_overflow", "MAESTRO_TEST_MAINTENANCE_INTERRUPT_READY="+readyPath)
	child.WaitDelay = 2 * time.Second
	sawReady := make(chan bool, 1)
	go func() {
		ticker := time.NewTicker(2 * time.Millisecond)
		defer ticker.Stop()
		for {
			if _, err := os.Stat(readyPath); err == nil {
				sawReady <- true
				cancel()
				return
			}
			select {
			case <-supervision.Done():
				sawReady <- false
				return
			case <-ticker.C:
			}
		}
	}()
	body, err := runMaintenanceChild(supervision, child)
	if !<-sawReady {
		t.Fatal("child did not finish overflowing both original bounded streams before interruption")
	}
	var failure *product.MaintenanceCommandError
	if !errors.As(err, &failure) || failure.Diagnostic.Code != "interrupted" || failure.Diagnostic.Phase != "execution" {
		t.Fatalf("original caller lost forced-interruption precedence: %v", err)
	}
	if len(body) != 0 {
		t.Fatal("interrupted overflowing child returned successful output")
	}
}

func TestMaintenanceOriginalChildOverflowWithoutInterruptionStaysRefused(t *testing.T) {
	child := exec.Command(os.Args[0], "-test.run=^TestMaintenanceCommandOutputHelper$")
	child.Env = append(os.Environ(), "MAESTRO_TEST_MAINTENANCE_OUTPUT=1")
	body, err := runMaintenanceChild(context.Background(), child)
	var failure *product.MaintenanceCommandError
	if !errors.As(err, &failure) || failure.Diagnostic.Code != "output_limit" || failure.Diagnostic.Phase != "output" || len(body) != 0 {
		t.Fatalf("ordinary overflowing child lost its refusal: %v", err)
	}
}
