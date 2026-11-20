//go:build !maestro_experimental_installed_seed

package main

import (
	"bytes"
	"context"
	"testing"
)

func TestProductionDoesNotExposeInstalledNativeHookAdmission(t *testing.T) {
	var output bytes.Buffer
	handled, err := experimentalEarlyCommand(context.Background(), "installed-native-hook", nil, &output)
	if handled || err != nil || output.Len() != 0 {
		t.Fatalf("production exposed experimental native admission: handled=%v err=%v output=%s", handled, err, output.String())
	}
}
