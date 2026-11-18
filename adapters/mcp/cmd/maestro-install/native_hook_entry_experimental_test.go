//go:build maestro_experimental_installed_seed

package main

import (
	"context"
	"testing"
)

func TestExperimentalNativeEntryRefusesMissingIndependentSeed(t *testing.T) {
	if handled, err := experimentalNativeHookCommand(context.Background(), "stable-native-hook", "not-executed", nil, nil); !handled || err == nil {
		t.Fatal("native route did not refuse missing seed")
	}
	if handled, err := experimentalNativeHookCommand(context.Background(), "unrelated", "not-executed", nil, nil); handled || err != nil {
		t.Fatal("native route consumed unrelated command")
	}
}
