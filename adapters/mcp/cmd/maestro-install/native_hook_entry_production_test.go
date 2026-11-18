//go:build !maestro_experimental_installed_seed

package main

import (
	"context"
	"testing"
)

func TestOrdinaryEntryDoesNotRecognizeExperimentalNativeRoute(t *testing.T) {
	if handled, err := experimentalNativeHookCommand(context.Background(), "stable-native-hook", "not-executed", []string{"--binding-sha256", "invalid"}, nil); handled || err != nil {
		t.Fatal("ordinary CLI consumed experimental native route")
	}
}
