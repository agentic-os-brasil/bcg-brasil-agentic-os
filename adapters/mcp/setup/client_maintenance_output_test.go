package setup

import (
	"bytes"
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
	if os.Getenv("MAESTRO_TEST_MAINTENANCE_OUTPUT") != "1" {
		return
	}
	if _, err := os.Stdout.Write(bytes.Repeat([]byte("a"), (1<<20)+33)); err != nil {
		os.Exit(2)
	}
	if _, err := os.Stderr.Write(bytes.Repeat([]byte("b"), (1<<20)+33)); err != nil {
		os.Exit(3)
	}
	os.Exit(0)
}
