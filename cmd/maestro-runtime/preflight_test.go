package main

import (
	"bytes"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/userlevel"
	"strings"
	"testing"
)

func TestPreflightReflectsEffectiveToken(t *testing.T) {
	var out, diagnostic bytes.Buffer
	code := run([]string{"preflight"}, strings.NewReader(""), &out, &diagnostic)
	if err := userlevel.EnsureNotElevated(); err != nil {
		if code != 2 || diagnostic.Len() == 0 || out.Len() != 0 {
			t.Fatalf("elevated preflight hid failure: %d %s %s", code, &out, &diagnostic)
		}
	} else if code != 0 || !strings.Contains(out.String(), `"state":"non_elevated"`) {
		t.Fatalf("unexpected preflight: %d %s %s", code, &out, &diagnostic)
	}
}
