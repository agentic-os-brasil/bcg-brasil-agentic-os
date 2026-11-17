package packaging

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEngineeringRuntimePilotAuthorityScope(t *testing.T) {
	now := time.Date(2026, 11, 16, 18, 0, 0, 0, time.UTC)
	public, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	const issuer = "maestro-engineering-pilot-20261116"
	const keyID = "pilot-20261116"
	entry := func() map[string]any {
		return map[string]any{"issuer": issuer, "key_id": keyID, "algorithm": "ed25519", "public_key": base64.StdEncoding.EncodeToString(public), "status": "active", "valid_from": now.Add(-time.Minute).Format(time.RFC3339), "valid_until": now.Add(14*24*time.Hour - time.Minute).Format(time.RFC3339)}
	}
	for _, tc := range []struct {
		name   string
		change func(map[string]any)
		want   bool
	}{
		{name: "short_lived_explicit_engineering_issuer", want: true},
		{name: "production_issuer", change: func(v map[string]any) { v["issuer"] = "maestro-release" }},
		{name: "expired", change: func(v map[string]any) { v["valid_until"] = now.Add(-time.Second).Format(time.RFC3339) }},
		{name: "long_lived", change: func(v map[string]any) { v["valid_until"] = now.Add(30 * 24 * time.Hour).Format(time.RFC3339) }},
		{name: "revoked", change: func(v map[string]any) {
			v["status"] = "revoked"
			v["revoked_at"] = now.Add(-time.Second).Format(time.RFC3339)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := entry()
			if tc.change != nil {
				tc.change(v)
			}
			body, err := json.Marshal(map[string]any{"schema_version": 1, "product": "maestro", "authorities": []any{v}})
			if err != nil {
				t.Fatal(err)
			}
			pin := fmt.Sprintf("%x", sha256.Sum256(body))
			expiry, err := engineeringRuntimeAuthority(body, pin, issuer, keyID, now)
			if (err == nil) != tc.want {
				t.Fatalf("engineering authority admission=%v, want=%v", err, tc.want)
			}
			if tc.want && !expiry.Equal(now.Add(14*24*time.Hour-time.Minute)) {
				t.Fatal("registry expiry was not preserved")
			}
			if _, err := engineeringRuntimeAuthority(body, strings.Repeat("0", 64), issuer, keyID, now); err == nil {
				t.Fatal("changed authority accepted")
			}
		})
	}
	body, _ := json.Marshal(map[string]any{"schema_version": 1, "product": "maestro", "authorities": []any{entry(), entry()}})
	if _, err := engineeringRuntimeAuthority(body, fmt.Sprintf("%x", sha256.Sum256(body)), issuer, keyID, now); err == nil {
		t.Fatal("multiple authority entries admitted to the closed pilot")
	}
}

func TestEngineeringRuntimeCarrierRequiresExactExecutingImage(t *testing.T) {
	root := t.TempDir()
	server := filepath.Join(root, "server", "maestro-mcp")
	binding := EngineeringRuntimeBinding{Profile: EngineeringRuntimeProfile, ClientVersion: "0.1.43", RegistrySHA256: strings.Repeat("a", 64), ManifestSHA256: strings.Repeat("b", 64), PackSHA256: strings.Repeat("c", 64), ServerSHA256: strings.Repeat("d", 64), Target: Target{OS: "darwin", Arch: "arm64"}, Issuer: "maestro-engineering-pilot-20261116", KeyID: "pilot-20261116", ExpiresAt: "2026-11-30T00:00:00Z"}
	status := PacketStatus{Version: binding.ClientVersion, RegistrySHA256: binding.RegistrySHA256, ServerSHA256: binding.ServerSHA256, Target: binding.Target, NativeMode: "mcp_only", DesktopPilot: &DesktopPilotBinding{Runtime: &binding, UserHome: filepath.Join(root, "owner")}}
	if err := ValidateEngineeringPacketSelection(root, status, server, binding.ServerSHA256, binding.ClientVersion, binding.RegistrySHA256); err != nil {
		t.Fatal("exact carrier rejected", err)
	}
	for _, tc := range []struct {
		name, path, image, version, registry string
	}{
		{"different_image_path", filepath.Join(root, "other", "maestro-mcp"), binding.ServerSHA256, binding.ClientVersion, binding.RegistrySHA256},
		{"changed_image_bytes", server, strings.Repeat("e", 64), binding.ClientVersion, binding.RegistrySHA256},
		{"unversioned_constructor", server, binding.ServerSHA256, "", binding.RegistrySHA256},
		{"changed_registry", server, binding.ServerSHA256, binding.ClientVersion, strings.Repeat("e", 64)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateEngineeringPacketSelection(root, status, tc.path, tc.image, tc.version, tc.registry); err == nil {
				t.Fatal("different constructor admitted")
			}
		})
	}
	status.NativeMode = "native"
	if err := ValidateEngineeringPacketSelection(root, status, server, binding.ServerSHA256, binding.ClientVersion, binding.RegistrySHA256); err == nil {
		t.Fatal("native carrier admitted as experimental MCP-only")
	}
}
