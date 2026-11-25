package main

import (
	"context"
	"testing"
)

func TestStableCheckLaunchDispatchRetainsIndependentAdmission(t *testing.T) {
	handled, err := runClients(context.Background(), "stable-check-launch", "unadmitted", lifecycleFlags{}, "", false)
	if !handled || err == nil {
		t.Fatalf("checker dispatch bypassed installed admission: handled=%v err=%v", handled, err)
	}
	handled, err = runClients(context.Background(), "stable-check-launch", "unadmitted", lifecycleFlags{ManifestSHA256: "unreviewed"}, "", false)
	if !handled || err == nil {
		t.Fatalf("checker dispatch accepted caller-selected release binding: handled=%v err=%v", handled, err)
	}
}
