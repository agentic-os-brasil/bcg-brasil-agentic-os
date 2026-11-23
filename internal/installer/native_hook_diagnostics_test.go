package installer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func TestNativeHookDiagnosticsNeverPrintPrivateErrorBodies(t *testing.T) {
	for _, phase := range []string{"parent_admission", "parent_child", "child_bootstrap", "child_admission", "handler"} {
		t.Run(phase, func(t *testing.T) {
			for _, test := range []struct {
				err     error
				outcome string
			}{{nil, "ok"}, {errors.New("PRIVATE_PROMPT /private/config SECRET_VALUE"), "unavailable"}, {fmt.Errorf("PRIVATE: %w", context.Canceled), "cancelled"}, {fmt.Errorf("PRIVATE: %w", context.DeadlineExceeded), "deadline"}} {
				var output bytes.Buffer
				now := time.Unix(0, 0)
				finish := nativeHookDiagnostic(phase, true, &output, func() time.Time { return now })
				now = now.Add(17 * time.Millisecond)
				finish(test.err)
				finish(errors.New("duplicate PRIVATE output"))
				want := "maestro-hook phase=" + phase + " state=start\nmaestro-hook phase=" + phase + " state=end elapsed_ms=17 outcome=" + test.outcome + "\n"
				if output.String() != want {
					t.Fatalf("metadata-only diagnostic changed: %q", output.String())
				}
			}
		})
	}
}

func TestNativeHookDiagnosticsOffOrUnknownPhaseWritesNothing(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, phase := range []string{"handler", "PRIVATE_PHASE\n/private/secret"} {
			if enabled && phase == "handler" {
				continue
			}
			var output bytes.Buffer
			finish := nativeHookDiagnostic(phase, enabled, &output, func() time.Time { t.Fatal("disabled diagnostics observed clock"); return time.Time{} })
			finish(errors.New(strings.Repeat("PRIVATE", 32)))
			if output.Len() != 0 {
				t.Fatal("disabled or open-ended phase emitted diagnostics")
			}
		}
	}
}

func TestNativeHookDiagnosticsRequireExactOptInOnStderr(t *testing.T) {
	for _, value := range []string{"", "true", "01", "1\nPRIVATE", "1"} {
		t.Run(fmt.Sprintf("value-%q", value), func(t *testing.T) {
			t.Setenv("MAESTRO_HOOK_DIAGNOSTICS", value)
			read, write, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			previous := os.Stderr
			os.Stderr = write
			finish := BeginNativeHookDiagnostic("handler")
			finish(errors.New("PRIVATE_ERROR /private/config"))
			os.Stderr = previous
			if err := write.Close(); err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(read)
			if closeErr := read.Close(); err != nil || closeErr != nil {
				t.Fatalf("read diagnostic pipe: %v %v", err, closeErr)
			}
			if value != "1" && len(body) != 0 {
				t.Fatalf("nonliteral opt-in emitted diagnostics: %q", body)
			}
			if value == "1" && (!strings.HasPrefix(string(body), "maestro-hook phase=handler state=start\n") || !strings.HasSuffix(string(body), " outcome=unavailable\n") || strings.Contains(string(body), "PRIVATE")) {
				t.Fatalf("public diagnostic entry exposed more than phase metadata: %q", body)
			}
		})
	}
}
