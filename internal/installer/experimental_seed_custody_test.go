package installer

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
)

func TestExperimentalSeedClassCannotEnterProductionAdmission(t *testing.T) {
	role, registry, key, now := experimentalRoleFixture(t)
	body, _ := json.Marshal(role)
	verified, err := VerifyExperimentalSeedAdmission(body, ed25519.Sign(key, body), registry, role.RegistrySHA256, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	status, err := ExperimentalSeedStatus(verified)
	if err != nil {
		t.Fatal(err)
	}
	engineering := status.(experimentalSeedStatus)
	manifest := releasecontract.Manifest{SchemaVersion: 2, Delivery: releasecontract.DeliveryComponent{InstallerVersion: role.InstallerVersion}}
	encoded, _ := json.Marshal(engineering)
	run := func(context.Context, string, ...string) ([]byte, error) { return encoded, nil }
	if _, err := readSeedStatus(context.Background(), "not-executed", run); err == nil {
		t.Fatal("ordinary decoder accepted engineering status")
	}
	if !experimentalStatusMatchesIdentity(engineering, verified.Identity()) {
		t.Fatal("engineering typed status differs")
	}

	production := seedStatus{SchemaVersion: 2, Product: "maestro", Component: "installer", Protocol: releasecontract.InstallerProtocol, InstallerVersion: role.InstallerVersion, AuthorityRegistrySHA256: role.RegistrySHA256}
	if !seedMatchesManifest(production, manifest, role.RegistrySHA256) {
		t.Fatal("production schema2 matching changed")
	}
	prodBytes, _ := json.Marshal(production)
	if _, err := readExperimentalSeedStatus(context.Background(), "not-executed", func(context.Context, string, ...string) ([]byte, error) { return prodBytes, nil }); err == nil {
		t.Fatal("engineering decoder accepted ordinary production status")
	}

	i := verified.Identity()
	plan := Plan{Profile: releasecontract.ModernProfile, NativeTrustMode: string(NativeTrustExperimentalInstalled), ExperimentalSeed: &i, ExperimentalBrainHome: filepath.Join(t.TempDir(), "brain")}
	if activationAdmissionMatches(plan, nil) {
		t.Fatal("engineering plan accepted without opaque role")
	}
	if !activationAdmissionMatches(plan, verified) {
		t.Fatal("engineering plan lost its explicit class")
	}
	plan.NativeTrustMode = string(NativeTrustStrict)
	if activationAdmissionMatches(plan, verified) || activationAdmissionMatches(plan, nil) {
		t.Fatal("engineering fields masqueraded as native strict")
	}
}

func TestExperimentalSeedCannotAdoptExistingProductionCustody(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skip("installed-seed pilot supports Mac and Windows")
	}
	root := t.TempDir()
	managed := filepath.Join(root, "managed")
	if err := os.Mkdir(managed, 0700); err != nil {
		t.Fatal(err)
	}
	name := "maestro-install"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	self := filepath.Join(managed, name)
	bytes := []byte("synthetic exact engineering installer, never executed")
	if err := os.WriteFile(self, bytes, 0700); err != nil {
		t.Fatal(err)
	}
	r, registry, key, now := experimentalRoleFixture(t)
	r.TargetOS, r.TargetArch = runtime.GOOS, runtime.GOARCH
	r.Source.OS, r.Source.Arch = runtime.GOOS, runtime.GOARCH
	r.InstallerSHA256 = fmt.Sprintf("%x", sha256.Sum256(bytes))
	body, _ := json.Marshal(r)
	verified, err := VerifyExperimentalSeedAdmission(body, ed25519.Sign(key, body), registry, r.RegistrySHA256, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(root, ".maestro-install")
	if err := os.Mkdir(data, 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(data, "production-marker.txt")
	if err := os.WriteFile(marker, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	op := SeededOperationOptions{ExperimentalSeed: verified, Self: self, Version: r.InstallerVersion, AuthorityRegistrySHA256: r.RegistrySHA256, UserHome: root, ManagedRoot: managed, DataRoot: data}
	if err := VerifyExperimentalInstalledBinding(op); err == nil {
		t.Fatal("production state adopted without reviewed engineering origin")
	}
	if b, err := os.ReadFile(marker); err != nil || string(b) != "preserve" {
		t.Fatal("failed admission changed existing state")
	}
	plan := Plan{UserHome: root, ManagedRoot: managed, DataRoot: data, Profile: releasecontract.ModernProfile, TargetOS: runtime.GOOS, TargetArch: runtime.GOARCH, NativeTrustMode: string(NativeTrustExperimentalInstalled)}
	i := verified.Identity()
	plan.ExperimentalSeed = &i
	plan.ExperimentalBrainHome = filepath.Join(root, "brain")
	plan.PlanDigest = PlanDigest(plan)
	if err := verifyExperimentalActivation(SeededActivationOptions{ExperimentalSeed: verified, Plan: plan}); err == nil || !strings.Contains(err.Error(), "existing installation") {
		t.Fatalf("first activation adopted existing custody: %v", err)
	}
	if _, err := os.Stat(filepath.Join(managed, "trust", experimentalSeedOriginName)); !os.IsNotExist(err) {
		t.Fatal("failed adoption initialized an origin")
	}
}

func experimentalCustodyFixture(t *testing.T) (ClientOptions, *time.Time, time.Time) {
	t.Helper()
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skip("installed-seed pilot supports Mac and Windows")
	}
	root := t.TempDir()
	user := filepath.Join(root, "user")
	managed := filepath.Join(root, "managed")
	for _, path := range []string{user, filepath.Join(managed, "trust")} {
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("MAESTRO_HOME", filepath.Join(user, ".maestro"))
	name := "maestro-install"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	self := filepath.Join(managed, name)
	seed := []byte("bounded synthetic installer fixture; never executed")
	if err := os.WriteFile(self, seed, 0700); err != nil {
		t.Fatal(err)
	}
	record, registry, key, now := experimentalRoleFixture(t)
	record.TargetOS, record.TargetArch = runtime.GOOS, runtime.GOARCH
	record.Source.OS, record.Source.Arch = runtime.GOOS, runtime.GOARCH
	record.InstallerSHA256 = fmt.Sprintf("%x", sha256.Sum256(seed))
	expires := now.Add(time.Minute)
	record.ExpiresAt = expires.Format(time.RFC3339)
	body, _ := json.Marshal(record)
	role, err := VerifyExperimentalSeedAdmission(body, ed25519.Sign(key, body), registry, record.RegistrySHA256, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	home, err := datahome.PreviewInstallation(datahome.Options{ProductRoot: managed, UserHome: user})
	if err != nil {
		t.Fatal(err)
	}
	i := role.Identity()
	plan := Plan{ExperimentalSeed: &i, ExperimentalBrainHome: home.Home, UserHome: user, ManagedRoot: managed, DataRoot: filepath.Join(user, ".maestro-install"), BootstrapperVersion: record.InstallerVersion, BootstrapperSHA256: record.InstallerSHA256, RegistrySHA256: record.RegistrySHA256, NativeTrustMode: string(NativeTrustExperimentalInstalled), TargetOS: runtime.GOOS, TargetArch: runtime.GOARCH, Profile: releasecontract.ModernProfile}
	plan.PlanDigest = PlanDigest(plan)
	if err := os.WriteFile(filepath.Join(managed, "trust", "release-authority-registry.json"), registry, 0600); err != nil {
		t.Fatal(err)
	}
	if err := stageExperimentalSeed(plan, role); err != nil {
		t.Fatal(err)
	}
	op := experimentalOperationOptions(plan, role)
	if err := recheckExperimentalOperation(op); err != nil {
		t.Fatal("fixture admission failed", err)
	}
	return ClientOptions{SeededOperationOptions: op, Request: ClientRequest{CarrierID: strings.Repeat("a", 32)}}, &now, expires
}

func TestExperimentalSeedExpiryAfterMaterializerDoesNotPublishCurrentRecord(t *testing.T) {
	o, now, expires := experimentalCustodyFixture(t)
	called := 0
	_, err := buildClientsWithSeedFence(context.Background(), o, installtx.State{}, ClientBinding{}, nil, func(context.Context, installtx.State, ClientBinding, *ClientRecord) (ClientMaterialization, error) {
		called++
		*now = expires.Add(time.Second)
		return ClientMaterialization{}, nil
	})
	if err == nil || called != 1 {
		t.Fatalf("expired materializer result escaped final fence: calls=%d err=%v", called, err)
	}
	if err := writeClientRecord(o.SeededOperationOptions, ClientRecord{CarrierID: o.Request.CarrierID}); err == nil {
		t.Fatal("expired result published a current record")
	}
	if _, err := os.Stat(o.DataRoot); !os.IsNotExist(err) {
		t.Fatalf("expired publication initialized private state: %v", err)
	}
}
