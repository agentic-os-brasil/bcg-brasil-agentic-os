package main

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestPublicMetadataHasClosedMembersAndBoundedTransport(t *testing.T) {
	allowed := map[string]bool{"N/release-manifest.json": true, "role.json": true}
	for _, tc := range []struct {
		name  string
		files []publicFile
		ok    bool
	}{
		{"exact", []publicFile{{"role.json", []byte("public")}, {"N/release-manifest.json", []byte("{}")}}, true},
		{"traversal", []publicFile{{"../role.json", []byte("x")}}, false},
		{"unlisted", []publicFile{{"credentials.json", []byte("x")}}, false},
		{"duplicate", []publicFile{{"role.json", []byte("x")}, {"role.json", []byte("x")}}, false},
		{"payload", []publicFile{{"role.json", []byte(strings.Repeat("x", 32769))}}, false},
		{"missing", []publicFile{{"role.json", []byte("x")}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := checkPublicFiles(tc.files, allowed); (err == nil) != tc.ok {
				t.Fatalf("accepted=%v err=%v", err == nil, err)
			}
		})
	}
}

// Synthetic transport/signature fixtures exercise no Windows/native/model qualification.
func fixture(t *testing.T) (options, receipt, publicMetadata) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "Developer")
	if e := os.Mkdir(root, 0700); e != nil {
		t.Fatal(e)
	}
	o := options{export: filepath.Join(root, "export"), revision: strings.Repeat("a", 40), tree: strings.Repeat("b", 40), run: "123", output: filepath.Join(root, "packed")}
	if e := os.Mkdir(o.export, 0700); e != nil {
		t.Fatal(e)
	}
	key := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{51}, 32))
	now := time.Now().UTC().Truncate(time.Second)
	registry, _ := json.Marshal(map[string]any{"schema_version": 1, "product": "maestro", "authorities": []any{map[string]any{"issuer": "maestro-engineering-pilot-20261119", "key_id": "pilot-20261119", "algorithm": "ed25519", "public_key": base64.StdEncoding.EncodeToString(key.Public().(ed25519.PublicKey)), "status": "active", "valid_from": now.Add(-time.Hour).Format(time.RFC3339), "valid_until": now.Add(24 * time.Hour).Format(time.RFC3339)}}})
	mustWrite := func(p string, b []byte) {
		t.Helper()
		if e := write(p, b); e != nil {
			t.Fatal(e)
		}
	}
	mustWrite(filepath.Join(o.export, "registry.json"), registry)
	bootstrap := []byte("synthetic PE transport only")
	mustWrite(filepath.Join(o.export, "bootstrap.exe"), bootstrap)
	pack := []byte("synthetic runtime transport only")
	r := receipt{SchemaVersion: 2, RuntimeProfile: "model-free-documents-v1", RuntimeLockSHA256: "38b1a0257c3feec8a5b4b97856e7f2096cdb72ed2bd1734d8e61567bf4757643", SourceRevision: o.revision, SourceTree: o.tree, ProducerRunID: o.run, Target: "windows/amd64", RuntimeSHA256: digest(pack), RegistrySHA256: digest(registry)}
	role := installer.ExperimentalSeedRecord{SchemaVersion: 1, Product: "maestro", Purpose: installer.ExperimentalSeedPurpose, Protocol: installer.ExperimentalSeedProtocol, Profile: installer.ExperimentalSeedProfile, RegistrySHA256: r.RegistrySHA256, InstallerVersion: "0.1.70", InstallerSHA256: digest(bootstrap), TargetOS: "windows", TargetArch: "amd64", Source: releasecontract.SourceTuple{SourceID: "engineering-windows", Channel: "canary", OS: "windows", Arch: "amd64"}, Issuer: "maestro-engineering-pilot-20261119", KeyID: "pilot-20261119", NotBefore: now.Add(-time.Hour).Format(time.RFC3339), ExpiresAt: now.Add(24 * time.Hour).Format(time.RFC3339)}
	p := publicMetadata{SchemaVersion: 1}
	var last releasecontract.Manifest
	for i, s := range []string{"N", "N1"} {
		version := []string{"0.1.70", "0.1.71"}[i]
		m := releasecontract.Manifest{SchemaVersion: 2, Product: "maestro", Release: version, Channel: "canary", Issuer: releasecontract.Issuer{ID: "maestro-release-candidate", KeyID: "candidate-unavailable"}, Server: releasecontract.ServerComponent{Version: version, CompatibleBundle: ">=0.1.0 <0.2.0", Protocol: releasecontract.ServerProtocol, EmbeddedInputsSHA256: strings.Repeat("a", 64), ExecutionContract: releasecontract.SyncedSourceContract}, Bundle: releasecontract.BundleComponent{Version: version, CompatibleServer: ">=0.1.0 <0.2.0"}, Delivery: releasecontract.DeliveryComponent{Profile: releasecontract.ModernProfile, InstallerVersion: "0.1.70", InstallerProtocol: releasecontract.InstallerProtocol}, Artifacts: []releasecontract.Artifact{}, Migrations: []releasecontract.Migration{}, ReleaseNotes: releasecontract.ReleaseNotes{Name: "notes-" + version + ".md", SHA256: digest([]byte("unsigned notes"))}}
		binding := installer.ExperimentalSeedRelease{Release: version}
		for _, a := range []struct {
			kind, os, arch, name string
			body                 []byte
		}{{"mcp_server", "windows", "amd64", "server-" + version + ".exe", []byte("synthetic server " + version)}, {"bundle", "any", "any", "bundle-" + version + ".tar.gz", []byte("synthetic bundle " + version)}, {"runtime_pack", "windows", "amd64", "runtime-" + version + ".tar.gz", pack}} {
			part := releasecontract.Artifact{Kind: a.kind, OS: a.os, Arch: a.arch, Name: a.name, SHA256: digest(a.body), Size: int64(len(a.body)), SignatureRef: a.name + ".sig"}
			switch a.kind {
			case "mcp_server":
				binding.ServerSHA256 = part.SHA256
			case "bundle":
				binding.BundleSHA256 = part.SHA256
			case "runtime_pack":
				part.Adapter = "docling"
				binding.RuntimeSHA256 = part.SHA256
			}
			m.Artifacts = append(m.Artifacts, part)
			mustWrite(filepath.Join(o.export, s, a.name), a.body)
			p.Files = append(p.Files, publicFile{s + "/" + part.SignatureRef, ed25519.Sign(key, a.body)})
		}
		var e error
		m.Delivery.ClientInputsSHA256, e = m.ClientInputsDigest()
		if e != nil {
			t.Fatal(e)
		}
		if e = m.Validate(); e != nil {
			t.Fatal(e)
		}
		b, _ := json.Marshal(m)
		mustWrite(filepath.Join(o.export, s, releaseverify.ManifestName), b)
		mustWrite(filepath.Join(o.export, s, m.ReleaseNotes.Name), []byte("unsigned notes"))
		m.Issuer = releasecontract.Issuer{ID: role.Issuer, KeyID: role.KeyID}
		m.ReleaseNotes.SHA256 = digest([]byte("signed notes"))
		b, _ = json.Marshal(m)
		p.Files = append(p.Files, publicFile{s + "/" + releaseverify.ManifestName, b}, publicFile{s + "/" + releaseverify.ManifestSignatureName, ed25519.Sign(key, b)}, publicFile{s + "/" + m.ReleaseNotes.Name, []byte("signed notes")})
		binding.ManifestSHA256 = digest(b)
		binding.ClientInputsSHA256 = m.Delivery.ClientInputsSHA256
		role.Releases = append(role.Releases, binding)
		last = m
	}
	rb, _ := json.Marshal(role)
	p.Files = append(p.Files, publicFile{"role.json", rb}, publicFile{"role.json.sig", ed25519.Sign(key, rb)})
	gen := releasecontract.Generation{SchemaVersion: 1, Product: "maestro", Protocol: releasecontract.GenerationProtocol, Issuer: last.Issuer, Source: role.Source, Sequence: 1, PublishedAt: now.Format(time.RFC3339), ExpiresAt: role.ExpiresAt, Release: last.Release, ManifestSHA256: role.Releases[1].ManifestSHA256, ClientInputsSHA256: last.Delivery.ClientInputsSHA256, Recalls: []releasecontract.ReleaseRecall{}, MinimumReaderContract: releasecontract.SyncedSourceContract}
	gb, _ := json.Marshal(gen)
	p.Files = append(p.Files, publicFile{"current.json", gb}, publicFile{"current.json.sig", ed25519.Sign(key, gb)})
	logText := "    native_windows_test.go:50: maestro_native_runtime_pack_sha256=" + r.RuntimeSHA256 + "\n"
	logText += "    native_windows_test.go:51: maestro_native_runtime_profile=model-free-documents-v1 lock_sha256=38b1a0257c3feec8a5b4b97856e7f2096cdb72ed2bd1734d8e61567bf4757643\n"
	for _, name := range []string{"TestWindowsNativeConfinement", "TestWindowsNativeConfinementDeadline", "TestWindowsSharedImplementationIdentityAcrossEntryModules", "TestWindowsRealAuthenticatedDocling"} {
		logText += "--- PASS: " + name + " (0.01s)\n"
	}
	for _, name := range []string{"sample.docx", "literal.txt", "structured.md", "quoted.csv", "generic.json", "generic.xml", "page.html", "page.htm", "spine.epub", "legacy.xls", "sheets.xlsx", "native.pdf", "native.pptx", "interactive.pdf", "nontext.pdf", "scanned.pdf", "mixed-image.pdf", "mixed.pdf", "protected.pdf", "pptx_alternate_presentation", "pptx_duplicate_slide_selection", "pptx_external_relationship", "pptx_macro_presentation", "pptx_metafile", "pptx_macro_payload", "pptx_DTD", "pptx_slide_type_confusion", "pptx_root_decoy", "validated_export", "installed_tamper", "missing_native_runtime"} {
		logText += "    --- PASS: TestWindowsRealAuthenticatedDocling/" + name + " (0.01s)\n"
	}
	log := []byte(logText) // Controlled proof-shaped text; no native execution is claimed.

	proof, _ := json.Marshal(map[string]any{"schema_version": 1, "qualification_profile": "windows_docling_documents_v1", "source_revision": o.revision, "inventory_sha256": strings.Repeat("c", 64), "test_binary_sha256": strings.Repeat("d", 64), "effective_sid_sha256": strings.Repeat("e", 64), "actual_standard_user": true, "elevated": false, "admin_member": false, "passed_tests": []string{"TestWindowsNativeConfinement", "TestWindowsNativeConfinementDeadline", "TestWindowsSharedImplementationIdentityAcrossEntryModules", "TestWindowsRealAuthenticatedDocling"}, "real_formats": 13, "runtime_profile": "model-free-documents-v1", "runtime_lock_sha256": "38b1a0257c3feec8a5b4b97856e7f2096cdb72ed2bd1734d8e61567bf4757643", "pdf_refusals": 6, "pptx_refusals": 9, "runtime_pack_sha256": r.RuntimeSHA256, "log_sha256": digest(log), "shared_implementation_attested": true, "mcp_server_execution": "not_attested", "native_client": "not_run", "publisher_authority": "not_granted"})
	mustWrite(filepath.Join(o.export, "proof.json"), proof)
	mustWrite(filepath.Join(o.export, "proof.log"), log)
	if e := filepath.WalkDir(o.export, func(p string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		b, e := read(p, 1<<20)
		if e != nil {
			return e
		}
		rel, _ := filepath.Rel(o.export, p)
		r.Files = append(r.Files, member{filepath.ToSlash(rel), int64(len(b)), digest(b)})
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	sort.Slice(r.Files, func(i, j int) bool { return r.Files[i].Path < r.Files[j].Path })
	b, _ := jsonBody(r)
	o.expected = digest(b)
	p.ExportSHA256 = o.expected
	mustWrite(filepath.Join(o.export, "export.json"), b)
	mustWrite(filepath.Join(o.export, "export.json.sha256"), []byte(o.expected+"  export.json\n"))
	return o, r, p
}
func TestWindowsPublicBridgeReconstructsExactSignedPair(t *testing.T) {
	o, r, p := fixture(t)
	if _, e := loadExport(o); e != nil {
		t.Fatal(e)
	}
	role, e := reconstruct(o, r, p, filepath.Join(filepath.Dir(o.export), "verified"))
	if e != nil {
		t.Fatal(e)
	}
	if role.Identity().TargetOS != "windows" {
		t.Fatal("wrong role")
	}
}
func TestWindowsPublicBridgeRejectsChangedAndForeignInputs(t *testing.T) {
	for _, mode := range []string{"changed-body", "mixed-run", "mixed-source", "unlisted-export", "foreign-pin", "tampered-signature", "changed-manifest", "foreign-role", "changed-generation", "unlisted-metadata", "reused-output"} {
		t.Run(mode, func(t *testing.T) {
			o, r, p := fixture(t)
			out := filepath.Join(filepath.Dir(o.export), "verified")
			load := false
			switch mode {
			case "changed-body":
				os.WriteFile(filepath.Join(o.export, "bootstrap.exe"), []byte("changed"), 0600)
				load = true
			case "mixed-run":
				o.run = "999"
				load = true
			case "mixed-source":
				o.tree = strings.Repeat("f", 40)
				load = true
			case "unlisted-export":
				os.WriteFile(filepath.Join(o.export, "private-profile.json"), []byte("{}"), 0600)
				load = true
			case "foreign-pin":
				o.expected = strings.Repeat("f", 64)
				load = true
			case "unlisted-metadata":
				p.Files = append(p.Files, publicFile{"credentials.json", []byte("{}")})
			case "reused-output":
				os.Mkdir(out, 0700)
				os.WriteFile(filepath.Join(out, "preserve"), []byte("foreign"), 0600)
			default:
				target := map[string]string{"tampered-signature": "N/" + releaseverify.ManifestSignatureName, "changed-manifest": "N/" + releaseverify.ManifestName, "foreign-role": "role.json", "changed-generation": "current.json"}[mode]
				for i := range p.Files {
					if p.Files[i].Path == target {
						p.Files[i].Body = append([]byte("changed"), p.Files[i].Body...)
					}
				}
			}
			var e error
			if load {
				_, e = loadExport(o)
			} else {
				_, e = reconstruct(o, r, p, out)
			}
			if e == nil {
				t.Fatal("unsafe input accepted")
			}
			if mode == "reused-output" {
				b, e := os.ReadFile(filepath.Join(out, "preserve"))
				if e != nil || string(b) != "foreign" {
					t.Fatal("foreign output changed")
				}
			}
		})
	}
}

func TestWindowsPublicBridgeCollectorEmitsOnlyBoundedPublicMetadata(t *testing.T) {
	o, r, p := fixture(t)
	v := filepath.Join(filepath.Dir(o.export), "signed")
	if _, e := reconstruct(o, r, p, v); e != nil {
		t.Fatal(e)
	}
	o.signedN = filepath.Join(v, "N")
	o.signedN1 = filepath.Join(v, "N1")
	o.role = filepath.Join(v, "role.json")
	o.signature = filepath.Join(v, "role.json.sig")
	o.source = filepath.Join(v, "source")
	if e := packMetadata(o); e != nil {
		t.Fatal(e)
	}
	encoded, e := read(filepath.Join(o.output, "public-metadata.base64"), maxEncodedPublicBytes)
	if e != nil {
		t.Fatal(e)
	}
	b, e := base64.StdEncoding.Strict().DecodeString(string(encoded))
	if e != nil {
		t.Fatal(e)
	}
	if len(b) > maxPublicBytes || len(encoded) > maxEncodedPublicBytes {
		t.Fatal("transport exceeds Windows/dispatch bound")
	}
	var got publicMetadata
	if e = releasecontract.DecodeStrictJSON(b, &got); e != nil {
		t.Fatal(e)
	}
	allowed, e := publicNames(o)
	if e != nil {
		t.Fatal(e)
	}
	if e = checkPublicFiles(got.Files, allowed); e != nil {
		t.Fatal(e)
	}
}
func TestWindowsNativeProofRequiresActualPackAndEveryFormat(t *testing.T) {
	o, _, _ := fixture(t)
	body, e := read(filepath.Join(o.export, "proof.json"), 64<<10)
	if e != nil {
		t.Fatal(e)
	}
	log, e := read(filepath.Join(o.export, "proof.log"), 4<<20)
	if e != nil {
		t.Fatal(e)
	}
	for _, mode := range []string{"missing-format", "skip", "wrong-pack", "no-standard-user", "legacy-profile", "legacy-lock", "old-format-count", "missing-pdf-positive", "missing-pptx-positive", "missing-pdf-refusal", "missing-pptx-refusal", "missing-profile-observation", "registry-read-experiment"} {
		t.Run(mode, func(t *testing.T) {
			var p map[string]any
			if e := json.Unmarshal(body, &p); e != nil {
				t.Fatal(e)
			}
			l := append([]byte(nil), log...)
			switch mode {
			case "missing-format":
				l = bytes.ReplaceAll(l, []byte("TestWindowsRealAuthenticatedDocling/sample.docx "), []byte("TestWindowsRealAuthenticatedDocling/not-docx "))
			case "skip":
				l = append(l, []byte("--- SKIP: extra\n")...)
			case "wrong-pack":
				p["runtime_pack_sha256"] = strings.Repeat("f", 64)
			case "no-standard-user":
				p["actual_standard_user"] = false
			case "legacy-profile":
				p["runtime_profile"] = "policy-formats-v1"
			case "legacy-lock":
				p["runtime_lock_sha256"] = "728aab0b59dd5011070dfc4e9159e3ed6180b1668539f84275f1b88e81f5be58"
			case "old-format-count":
				p["real_formats"] = 11
			case "missing-pdf-positive":
				l = bytes.ReplaceAll(l, []byte("TestWindowsRealAuthenticatedDocling/native.pdf "), []byte("TestWindowsRealAuthenticatedDocling/no-pdf "))
			case "missing-pptx-positive":
				l = bytes.ReplaceAll(l, []byte("TestWindowsRealAuthenticatedDocling/native.pptx "), []byte("TestWindowsRealAuthenticatedDocling/no-pptx "))
			case "missing-pdf-refusal":
				l = bytes.ReplaceAll(l, []byte("TestWindowsRealAuthenticatedDocling/protected.pdf "), []byte("TestWindowsRealAuthenticatedDocling/no-refusal "))
			case "missing-pptx-refusal":
				l = bytes.ReplaceAll(l, []byte("TestWindowsRealAuthenticatedDocling/pptx_macro_payload "), []byte("TestWindowsRealAuthenticatedDocling/no-refusal "))
			case "registry-read-experiment":
				p["qualification_profile"] = "windows_docling_registry_read_r17"
			case "missing-profile-observation":
				l = bytes.ReplaceAll(l, []byte("maestro_native_runtime_profile="), []byte("not-a-profile-observation="))

			}
			p["log_sha256"] = digest(l)
			b, _ := json.Marshal(p)
			if _, e := verifyProof(b, l, o.revision); e == nil {
				t.Fatal("invalid native proof admitted")
			}
		})
	}
}

func TestWindowsKitZipKeepsOnlyExpectedFactoryBytes(t *testing.T) {
	for _, mode := range []string{"exact", "unlisted", "changed"} {
		t.Run(mode, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "kit")
			if e := os.Mkdir(root, 0700); e != nil {
				t.Fatal(e)
			}
			body := []byte("public factory bytes")
			if e := write(filepath.Join(root, "Maestro.cmd"), body); e != nil {
				t.Fatal(e)
			}
			want := map[string]string{"Maestro.cmd": digest(body)}
			if mode == "unlisted" {
				if e := write(filepath.Join(root, "private.json"), []byte("preserve")); e != nil {
					t.Fatal(e)
				}
			}
			if mode == "changed" {
				if e := os.WriteFile(filepath.Join(root, "Maestro.cmd"), []byte("changed"), 0600); e != nil {
					t.Fatal(e)
				}
			}
			out := root + ".zip"
			e := zipKit(root, out, want)
			if (e == nil) != (mode == "exact") {
				t.Fatalf("mode %s err %v", mode, e)
			}
			if mode == "exact" {
				z, e := zip.OpenReader(out)
				if e != nil {
					t.Fatal(e)
				}
				defer z.Close()
				if len(z.File) != 1 || z.File[0].Name != "kit/Maestro.cmd" {
					t.Fatal("ZIP closure differs")
				}
			}
			if mode == "unlisted" {
				b, e := os.ReadFile(filepath.Join(root, "private.json"))
				if e != nil || string(b) != "preserve" {
					t.Fatal("foreign member changed")
				}
				if _, e = os.Stat(out); !os.IsNotExist(e) {
					t.Fatal("unlisted output created ZIP")
				}
			}
		})
	}
}
