package installer

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestExperimentalInstalledContextCanceledBeforeOrigin(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := LoadExperimentalInstalledContextWithContext(ctx, "/synthetic/not-an-origin", "", time.Now)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("installed context loader dropped cancellation: %v", err)
	}
}
