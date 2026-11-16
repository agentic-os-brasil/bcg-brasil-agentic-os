package main

import (
	"os"
	"strings"
	"testing"
)

func TestOwnerReviewChannelIsStartupOnly(t *testing.T) {
	original := os.Args
	defer func() { os.Args = original }()
	os.Args = []string{"maestro-mcp", "serve", "--owner-review-channel", "model-approved"}
	if err := run(); err == nil || err.Error() != "owner_review_channel_unavailable" {
		t.Fatalf("unknown startup channel: %v", err)
	}
	os.Args = []string{"maestro-mcp", "memory-apply", "--owner-review-channel", "local-browser"}
	if err := run(); err == nil || !strings.Contains(err.Error(), "flag provided but not defined") {
		t.Fatalf("domain command acquired browser startup authority: %v", err)
	}
}
