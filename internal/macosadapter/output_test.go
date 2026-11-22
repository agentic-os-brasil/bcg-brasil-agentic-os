package macosadapter

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestBoundedBufferDrainsCopiedOutput(t *testing.T) {
	for _, size := range []int{0, 8191, 8192, 8193, 32785} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			input := ("begin\n" + strings.Repeat("0123456789abcdef", (size+15)/16))[:size]
			source := bytes.NewReader([]byte(input))
			var output boundedBuffer
			copied, err := io.Copy(&output, source)
			if err != nil || copied != int64(size) || source.Len() != 0 {
				t.Fatalf("output was not fully drained: copied=%d remaining=%d err=%v", copied, source.Len(), err)
			}
			if got, want := output.String(), input[:min(size, 8192)]; got != want {
				t.Fatalf("captured %d bytes, want the first %d bytes", len(got), len(want))
			}
		})
	}
}

func TestBoundedBufferDrainsExecOutput(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestBoundedBufferOutputProcess$")
	command.Env = append(os.Environ(), "MAESTRO_TEST_BOUNDED_OUTPUT_PROCESS=1")
	var output boundedBuffer
	command.Stdout, command.Stderr = &output, &output
	if err := command.Run(); err != nil {
		t.Fatalf("output-producing process did not finish: %v", err)
	}
	want := ("stderr-prefix\nstdout-prefix\n" + strings.Repeat("A", 8192))[:8192]
	if got := output.String(); got != want {
		t.Fatalf("captured %d bytes, want the first %d bytes of combined output", len(got), len(want))
	}
}

func TestBoundedBufferOutputProcess(t *testing.T) {
	if os.Getenv("MAESTRO_TEST_BOUNDED_OUTPUT_PROCESS") != "1" {
		return
	}
	for _, output := range []struct {
		writer io.Writer
		text   string
	}{
		{os.Stderr, "stderr-prefix\n"},
		{os.Stdout, "stdout-prefix\n"},
		{os.Stdout, strings.Repeat("A", 512*1024)},
		{os.Stderr, strings.Repeat("B", 512*1024)},
	} {
		if _, err := io.WriteString(output.writer, output.text); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCapturedIdleRejectsTruncatedOutput(t *testing.T) {
	prefix := "\"HIDIdleTime\" = 600000000000\n"
	full := prefix + strings.Repeat(" ", 8192-len(prefix))
	for _, test := range []struct {
		name       string
		input      string
		writeAgain bool
		extra      []byte
		wantError  bool
	}{
		{name: "below_limit", input: prefix},
		{name: "exact_limit", input: full},
		{name: "duplicate_counter_after_limit", input: full + "\n\"HIDIdleTime\" = 0\n", wantError: true},
		{name: "crossing_limit", input: full + "x", wantError: true},
		{name: "empty_write_at_limit", input: full, writeAgain: true},
		{name: "write_after_limit", input: full, writeAgain: true, extra: []byte("hidden"), wantError: true},
		{name: "empty_write_keeps_overflow", input: full + "x", writeAgain: true, wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output boundedBuffer
			copied, err := io.Copy(&output, bytes.NewReader([]byte(test.input)))
			if err != nil || copied != int64(len(test.input)) {
				t.Fatalf("capture did not drain: copied=%d err=%v", copied, err)
			}
			if test.writeAgain {
				if consumed, err := output.Write(test.extra); err != nil || consumed != len(test.extra) {
					t.Fatalf("capture did not consume subsequent write: consumed=%d err=%v", consumed, err)
				}
			}
			state, idleFor, err := classifyIdleCapture(&output, 5*time.Minute)
			if test.wantError {
				if err == nil || state != "" || idleFor != 0 {
					t.Fatalf("truncated capture became idle evidence: state=%q idle=%s err=%v", state, idleFor, err)
				}
			} else if err != nil || state != NativeIdleConfirmed || idleFor != 10*time.Minute {
				t.Fatalf("complete capture rejected: state=%q idle=%s err=%v", state, idleFor, err)
			}
		})
	}
}
