//go:build windows

package darwin_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/darwin"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"golang.org/x/sys/windows"
)

// The existing unsafe-mode case exercises a genuine public-read ACL on Windows,
// rather than treating POSIX mode bits as Windows access-control evidence.
func exposeProductCache(path string) error {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;" + user.User.Sid.String() + ")(A;;FR;;;WD)")
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
}

// The descriptor stays only in this owned-fixture assertion, never in evidence.
func productCacheProtection(path string) (string, error) {
	descriptor, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return "", err
	}
	return descriptor.String(), nil
}

type retainedProductProbe struct {
	maintenance.DarwinBinding
	inside *bool
}

func (p retainedProductProbe) Handler(job string, source *maintenance.DarwinSource, attended bool) maintenance.Handler {
	original := p.DarwinBinding.Handler(job, source, attended)
	return maintenance.HandlerFunc(func(ctx context.Context, command maintenance.Command, grant maintenance.ExecutionGrant) (maintenance.HandlerResult, error) {
		*p.inside = true
		defer func() { *p.inside = false }()
		return original.ExecuteAuthorized(ctx, command, grant)
	})
}

func TestProductDarwinWindowsRetainedTransaction(t *testing.T) {
	var root string
	inside := false
	selected := ""
	attempted := map[string]bool{}
	probedParents := map[string]bool{}
	s, _, now := productServiceWith(t, func(options *maintenance.ServiceOptions) {
		options.DarwinFactory = func(scope maintenance.DarwinScope) (maintenance.DarwinBinding, error) {
			root = scope.Root
			binding, err := darwin.NewProductBinding(scope)
			return retainedProductProbe{binding, &inside}, err
		}
		options.InstalledGuard = func(publish func() error) error {
			if inside && !attempted[selected] {
				parents := []string{"managed-state", "reports", filepath.Join("reports", selected, "versions")}
				if selected != maintenance.DarwinDailyJobID {
					parents = append(parents, "proposals")
				}
				ready := true
				for _, parent := range parents {
					if _, err := os.Stat(filepath.Join(root, parent)); err != nil {
						ready = false
					}
				}
				if ready {
					attempted[selected] = true
					for _, parent := range parents {
						probedParents[parent] = true
						path := filepath.Join(root, parent)
						if err := os.Rename(path, path+"-moved"); err == nil {
							t.Errorf("live namespace parent moved: %s", parent)
							return errors.New("retained Darwin namespace was lost")
						}
					}
				}
			}
			return publish()
		}
	})
	for _, job := range []string{maintenance.DarwinDailyJobID, maintenance.DarwinWeeklyJobID, maintenance.DarwinMonthlyJobID} {
		selected = job
		*now = now.Add(time.Second)
		qualified := qualify(t, s, job)
		if qualified.Qualification == nil {
			t.Fatalf("actual transaction did not qualify: %s", job)
		}
		report, err := s.Report(job)
		if err != nil || report.Darwin == nil || report.Darwin.RecoveryState != "" {
			t.Fatalf("transaction readback: %s %+v %v", job, report, err)
		}
		evidence := report.Darwin.Evidence
		if job == maintenance.DarwinDailyJobID {
			if evidence.Effect != "initialized" || evidence.Before.Present || !evidence.After.Present {
				t.Fatalf("first cache transition missing: %+v", evidence)
			}
		} else if evidence.Outcome != maintenance.ReceiptProposalEmitted || evidence.ProposalCount < 1 {
			t.Fatalf("immutable assessment missing: %+v", evidence)
		}
		pointer, err := os.ReadFile(filepath.Join(root, "reports", job, "current.json"))
		if err != nil {
			t.Fatal(err)
		}
		var current struct{ Occurrence string }
		if json.Unmarshal(pointer, &current) != nil || current.Occurrence != evidence.OccurrenceDigest {
			t.Fatalf("current evidence pointer diverged: %s", job)
		}
		if _, err = os.Stat(filepath.Join(root, "reports", job, "versions", current.Occurrence+".json")); err != nil {
			t.Fatal(err)
		}
	}
	if len(attempted) != 3 {
		t.Fatalf("test did not cross every live retained transaction: %v", attempted)
	}
	if _, err := os.Stat(filepath.Join(root, "managed-state", "recovery.json")); err != nil {
		t.Fatal("original journal missing", err)
	}
	// The same exact names become movable after release. This distinguishes
	// retained namespace custody from unrelated permanent ACL refusal.
	for parent := range probedParents {
		path := filepath.Join(root, parent)
		moved := path + "-released"
		if _, err := os.Lstat(moved); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("release control destination is not absent", err)
		}
		if err := os.Rename(path, moved); err != nil {
			t.Fatalf("released namespace parent cannot move: %s %v", parent, err)
		}
		if err := os.Rename(moved, path); err != nil {
			t.Fatalf("owned release control could not restore its fixture: %v", err)
		}
	}
	if err := os.Rename(root, root+"-closed"); err != nil {
		t.Fatalf("transaction leaked retained handles: %v", err)
	}
	for _, kind := range []string{"reparse_artifact", "reparse_proposal_directory", "reparse_ancestor"} {
		t.Run(kind, func(t *testing.T) { productReparseRefusal(t, kind) })
	}
}

// These are original product reads/approved attempts, not legacy Canary
// persistence. Every reparse fixture and assertion is required on Windows.
func productReparseRefusal(t *testing.T, kind string) {
	t.Helper()
	var root string
	var binding maintenance.DarwinBinding
	s, _, _ := productServiceWith(t, func(options *maintenance.ServiceOptions) {
		options.DarwinFactory = func(scope maintenance.DarwinScope) (maintenance.DarwinBinding, error) {
			root = scope.Root
			var err error
			binding, err = darwin.NewProductBinding(scope)
			return binding, err
		}
	})
	outside := t.TempDir()
	if err := scheduler.EnsurePrivateDirectory(outside); err != nil {
		t.Fatal(err)
	}
	if kind == "reparse_ancestor" {
		ancestor := filepath.Dir(root)
		if _, err := os.Lstat(ancestor); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("ancestor fixture is not absent", err)
		}
		if err := scheduler.EnsurePrivateDirectory(filepath.Dir(ancestor)); err != nil {
			t.Fatal(err)
		}
		review, err := s.Review(maintenance.Operation{Kind: "qualify", JobID: maintenance.DarwinDailyJobID})
		if err != nil {
			t.Fatal("unaliased product review failed", err)
		}
		before := productAliasTargetState(t, outside)
		productDirectoryReparse(t, ancestor, outside)
		if _, err := s.Apply(context.Background(), review, func(context.Context, maintenance.ProductReview) error { return nil }); err == nil {
			t.Fatal("product initialized through a reparse ancestor")
		}
		if productAliasTargetState(t, outside) != before {
			t.Fatal("ancestor refusal modified its target")
		}
		return
	}
	qualify(t, s, maintenance.DarwinWeeklyJobID)
	report, err := binding.Report(maintenance.DarwinWeeklyJobID)
	if err != nil || report.Evidence.ProposalArtifactID == "" {
		t.Fatal("original proposal readback failed", err)
	}
	if kind == "reparse_artifact" {
		artifact := filepath.Join(root, "proposals", report.Evidence.ProposalArtifactID+".json")
		target := filepath.Join(outside, "original.json")
		if err := os.Rename(artifact, target); err != nil {
			t.Fatal(err)
		}
		before := productAliasTargetState(t, outside)
		expected, err := os.ReadFile(target)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, artifact); err != nil {
			t.Fatal("required real file reparse fixture unavailable", err)
		}
		productAssertReparse(t, artifact, false)
		// Ordinary access still reaches the exact previously validated bytes;
		// the domain denial cannot be explained by missing data or read ACL.
		actual, err := os.ReadFile(artifact)
		if err != nil || string(actual) != string(expected) {
			t.Fatal("file reparse read control changed or became unreadable", err)
		}
		if _, err := binding.Evidence(maintenance.DarwinWeeklyJobID, report.Evidence.OccurrenceDigest); err == nil {
			t.Fatal("product read followed a reparse artifact")
		}
		if productAliasTargetState(t, outside) != before {
			t.Fatal("artifact refusal modified its target")
		}
		return
	}
	if kind != "reparse_proposal_directory" {
		t.Fatal("unknown product reparse case")
	}
	// Review a fresh monthly operation before the single namespace change.
	// Existing weekly qualification cannot satisfy this attempted publication.
	review, err := s.Review(maintenance.Operation{Kind: "qualify", JobID: maintenance.DarwinMonthlyJobID})
	if err != nil {
		t.Fatal("unaliased monthly review failed", err)
	}
	beforeSource, err := binding.Inspect(maintenance.DarwinMonthlyJobID)
	if err != nil {
		t.Fatal(err)
	}
	proposals := filepath.Join(root, "proposals")
	target := filepath.Join(outside, "original-proposals")
	if err := os.Rename(proposals, target); err != nil {
		t.Fatal(err)
	}
	before := productAliasTargetState(t, outside)
	productDirectoryReparse(t, proposals, target)
	afterSource, err := binding.Inspect(maintenance.DarwinMonthlyJobID)
	if err != nil || !reflect.DeepEqual(afterSource, beforeSource) {
		t.Fatal("proposal reparse unexpectedly changed the source-control input", err)
	}
	if _, err := os.ReadFile(filepath.Join(proposals, report.Evidence.ProposalArtifactID+".json")); err != nil {
		t.Fatal("junction read control became inaccessible", err)
	}
	if _, err := s.Apply(context.Background(), review, func(context.Context, maintenance.ProductReview) error { return nil }); err == nil {
		t.Fatal("product publication traversed a reparse proposal directory")
	}
	if productAliasTargetState(t, outside) != before {
		t.Fatal("proposal-directory refusal modified its target")
	}
}

func productDirectoryReparse(t *testing.T, alias, target string) {
	t.Helper()
	shell := filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")
	if out, err := exec.Command(shell, "/c", "mklink", "/J", alias, target).CombinedOutput(); err != nil {
		t.Fatalf("required ordinary-user junction fixture unavailable: %s %v", out, err)
	}
	productAssertReparse(t, alias, true)
}

func productAssertReparse(t *testing.T, path string, directory bool) {
	t.Helper()
	nativePath := path
	if filepath.IsAbs(path) && len(filepath.VolumeName(path)) == 2 {
		nativePath = `\\?\` + path
	}
	name, err := windows.UTF16PtrFromString(nativePath)
	if err != nil {
		t.Fatal(err)
	}
	attributes, err := windows.GetFileAttributes(name)
	if err != nil || attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT == 0 || (attributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) != directory {
		t.Fatalf("fixture is not the required native reparse class: directory=%t err=%v", directory, err)
	}
}

// All descriptor/path/body material remains in this owned-fixture assertion.
// The digest is compared only in memory and never emitted as evidence.
func productAliasTargetState(t *testing.T, root string) [32]byte {
	t.Helper()
	state := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("unexpected alias inside owned target")
		}
		name, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		state[name+":mode"] = info.Mode().String()
		state[name+":protection"], err = productCacheProtection(path)
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			if !info.Mode().IsRegular() {
				return errors.New("nonregular owned target")
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			state[name+":body"] = string(body)
		}
		return nil
	})
	if err != nil {
		t.Fatal("owned target snapshot failed", err)
	}
	body, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(body)
}
