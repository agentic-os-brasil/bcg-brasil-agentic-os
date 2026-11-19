// Development-only transport for native Windows pilot construction. Signing stays off-host.
package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/dev/releasepack"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest/docling"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/pilotdocument"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseprovider"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

const maxPublicBytes = 24000 // Outer base64 fits both the Windows 32,767-character variable and 65,535-character dispatch.
const maxEncodedPublicBytes = 32000

var shaPattern = regexp.MustCompile("^[a-f0-9]{64}$")
var revisionPattern = regexp.MustCompile("^[a-f0-9]{40}$")
var runPattern = regexp.MustCompile("^[1-9][0-9]{0,19}$")

type member struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}
type receipt struct {
	SchemaVersion  int      `json:"schema_version"`
	SourceRevision string   `json:"source_revision"`
	SourceTree     string   `json:"source_tree"`
	ProducerRunID  string   `json:"producer_run_id"`
	Target         string   `json:"target"`
	RuntimeSHA256  string   `json:"runtime_sha256"`
	RegistrySHA256 string   `json:"registry_sha256"`
	Files          []member `json:"files"`
}
type publicFile struct {
	Path string `json:"path"`
	Body []byte `json:"body"`
}
type publicMetadata struct {
	SchemaVersion int          `json:"schema_version"`
	ExportSHA256  string       `json:"export_sha256"`
	Files         []publicFile `json:"files"`
}
type options struct{ mode, root, output, raw, registry, proof, proofLog, n, n1, installerVersion, export, expected, source, signedN, signedN1, role, signature, metadata, revision, tree, run string }

func main() {
	var o options
	flag.StringVar(&o.mode, "mode", "", "export, metadata or assemble")
	for _, f := range []struct {
		p    *string
		name string
	}{
		{&o.root, "root"}, {&o.output, "output"}, {&o.raw, "raw-inputs"}, {&o.registry, "registry"}, {&o.proof, "proof"}, {&o.proofLog, "proof-log"},
		{&o.n, "version"}, {&o.n1, "next-version"}, {&o.installerVersion, "installer-version"}, {&o.export, "export"}, {&o.expected, "export-sha256"},
		{&o.source, "source"}, {&o.signedN, "signed-n"}, {&o.signedN1, "signed-n1"}, {&o.role, "role"}, {&o.signature, "role-signature"}, {&o.metadata, "metadata"},
		{&o.revision, "source-revision"}, {&o.tree, "source-tree"}, {&o.run, "producer-run-id"}} {
		flag.StringVar(f.p, f.name, "", "explicit "+f.name)
	}
	flag.Parse()
	var err error
	if flag.NArg() != 0 {
		err = errors.New("unexpected positional arguments")
	} else {
		switch o.mode {
		case "export":
			err = exportNative(o)
		case "metadata":
			err = packMetadata(o)
		case "assemble":
			err = assembleNative(o)
		default:
			err = errors.New("explicit export, metadata or assemble mode required")
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func native() error {
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		return errors.New("matching native Windows amd64 runner required")
	}
	return nil
}
func checkNativeSource(o options) error {
	for _, a := range []struct {
		args []string
		want string
	}{
		{[]string{"rev-parse", "HEAD"}, o.revision}, {[]string{"rev-parse", "HEAD^{tree}"}, o.tree}, {[]string{"status", "--porcelain"}, ""},
	} {
		c := exec.Command("git", append([]string{"-C", o.root}, a.args...)...)
		b, e := c.Output()
		if e != nil {
			return e
		}
		if strings.TrimSpace(string(b)) != a.want {
			return errors.New("native builder source differs from the exact clean reviewed tree")
		}
	}
	return nil
}
func digest(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
func read(p string, max int64) ([]byte, error) {
	f, e := scheduler.OpenRegularFile(p)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	if e = scheduler.ValidateSingleLink(f); e != nil {
		return nil, e
	}
	before, e := f.Stat()
	if e != nil {
		return nil, e
	}
	if before.Size() < 0 || before.Size() > max {
		return nil, errors.New("bounded ordinary input required")
	}
	b, e := io.ReadAll(io.LimitReader(f, max+1))
	if e != nil {
		return nil, e
	}
	after, e := f.Stat()
	if e != nil {
		return nil, e
	}
	current, e := os.Lstat(p)
	if e != nil {
		return nil, e
	}
	if int64(len(b)) != before.Size() || after.Size() != before.Size() || !before.ModTime().Equal(after.ModTime()) || !os.SameFile(after, current) || !current.Mode().IsRegular() {
		return nil, errors.New("input changed during observation")
	}
	return b, nil
}
func write(p string, b []byte) error {
	if e := os.MkdirAll(filepath.Dir(p), 0700); e != nil {
		return e
	}
	if e := scheduler.ValidateDirectory(filepath.Dir(p)); e != nil {
		return e
	}
	f, e := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	_, e = f.Write(b)
	ce := f.Close()
	if e != nil {
		return e
	}
	return ce
}
func fresh(p string) error {
	if !filepath.IsAbs(p) || filepath.Clean(p) != p || !strings.Contains(filepath.ToSlash(p), "/Developer/") {
		return errors.New("absolute clean physical Developer output required")
	}
	parent, e := filepath.EvalSymlinks(filepath.Dir(p))
	if e != nil {
		return e
	}
	if parent != filepath.Dir(p) {
		return errors.New("output parent is a link")
	}
	if e = scheduler.ValidateDirectory(parent); e != nil {
		return e
	}
	return os.Mkdir(p, 0700) // Existing and interrupted outputs are retained and refused.
}
func jsonBody(v any) ([]byte, error) {
	b, e := json.MarshalIndent(v, "", "  ")
	return append(b, '\n'), e
}
func manifest(p string) (releasecontract.Manifest, error) {
	var m releasecontract.Manifest
	b, e := read(filepath.Join(p, releaseverify.ManifestName), 1<<20)
	if e != nil {
		return m, e
	}
	e = releasecontract.DecodeStrictJSON(b, &m)
	if e == nil {
		e = m.Validate()
	}
	return m, e
}
func candidateMembers(m releasecontract.Manifest) []string {
	v := []string{releaseverify.ManifestName, m.ReleaseNotes.Name}
	for _, a := range m.Artifacts {
		v = append(v, a.Name)
	}
	return v
}
func checkNames(root string, want []string) error {
	entries, e := os.ReadDir(root)
	if e != nil {
		return e
	}
	if len(entries) != len(want) {
		return errors.New("closed directory membership differs")
	}
	allowed := map[string]bool{}
	for _, n := range want {
		allowed[n] = true
	}
	for _, v := range entries {
		if !allowed[v.Name()] || v.IsDir() || v.Type()&os.ModeSymlink != 0 {
			return errors.New("unlisted or nonregular directory member")
		}
	}
	return nil
}
func copyOne(from, to string, max int64) error {
	b, e := read(from, max)
	if e != nil {
		return e
	}
	return write(to, b)
}
func exportNative(o options) error {
	if e := native(); e != nil {
		return e
	}
	if e := checkNativeSource(o); e != nil {
		return e
	}
	if !revisionPattern.MatchString(o.revision) || !revisionPattern.MatchString(o.tree) || !runPattern.MatchString(o.run) || o.n == o.n1 {
		return errors.New("exact distinct versions, source and producer identity required")
	}
	if _, e := releasecontract.ParseVersionRange(">=" + o.n + " <" + o.n1); e != nil {
		return e
	}
	p, err := read(o.proof, 64<<10)
	if err != nil {
		return err
	}
	log, err := read(o.proofLog, 4<<20)
	if err != nil {
		return err
	}
	proof, err := verifyProof(p, log, o.revision)
	if err != nil {
		return err
	}
	if err = fresh(o.output); err != nil {
		return err
	}
	assembly, err := docling.AssembleProfile(o.raw, filepath.Join(o.output, "construction-runtime"), "windows", "amd64", docling.LegacyProfile)
	if err != nil {
		return err
	}
	if assembly.ArchiveSHA256 != proof {
		return errors.New("constructed pack differs from the actual ordinary-user converter proof")
	}
	reg, err := read(o.registry, 1<<20)
	if err != nil {
		return err
	}
	r := receipt{SchemaVersion: 1, SourceRevision: o.revision, SourceTree: o.tree, ProducerRunID: o.run, Target: "windows/amd64", RuntimeSHA256: proof, RegistrySHA256: digest(reg)}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	for i, v := range []string{o.n, o.n1} {
		slot := []string{"N", "N1"}[i]
		result, e := releasepack.BuildModernCandidate(ctx, releasepack.ModernOptions{Root: o.root, Output: filepath.Join(o.output, "construction-"+slot), Version: v, InstallerVersion: o.installerVersion, Channel: "canary", AuthorityRegistry: o.registry, Target: releasepack.Target{OS: "windows", Arch: "amd64"}, DoclingPack: assembly.ArchivePath, ExperimentalInstalledSeed: true})
		if e != nil {
			return e
		}
		for _, name := range candidateMembers(result.Manifest) {
			if e = copyOne(filepath.Join(result.Directory, name), filepath.Join(o.output, "unsigned", slot, name), 1<<30); e != nil {
				return e
			}
		}
		if i == 0 {
			if e = copyOne(result.Installer, filepath.Join(o.output, "unsigned", "bootstrap.exe"), 256<<20); e != nil {
				return e
			}
		} else {
			a, e := read(result.Installer, 256<<20)
			if e != nil {
				return e
			}
			b, e := read(filepath.Join(o.output, "unsigned", "bootstrap.exe"), 256<<20)
			if e != nil {
				return e
			}
			if !bytes.Equal(a, b) {
				return errors.New("release pair does not share one identical stable bootstrapper")
			}
		}
	}
	out := filepath.Join(o.output, "unsigned")
	for name, b := range map[string][]byte{"registry.json": reg, "proof.json": p, "proof.log": log} {
		if err = write(filepath.Join(out, name), b); err != nil {
			return err
		}
	}
	err = filepath.WalkDir(out, func(p string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		b, e := read(p, 1<<30)
		if e != nil {
			return e
		}
		rel, e := filepath.Rel(out, p)
		if e != nil {
			return e
		}
		r.Files = append(r.Files, member{filepath.ToSlash(rel), int64(len(b)), digest(b)})
		return nil
	})
	if err != nil {
		return err
	}
	sort.Slice(r.Files, func(i, j int) bool { return r.Files[i].Path < r.Files[j].Path })
	body, err := jsonBody(r)
	if err != nil {
		return err
	}
	if err = write(filepath.Join(out, "export.json"), body); err != nil {
		return err
	}
	if err = write(filepath.Join(out, "export.json.sha256"), []byte(digest(body)+"  export.json\n")); err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]string{"export": out, "export_sha256": digest(body), "evidence": "native_unsigned_only"})
}
func verifyProof(body, log []byte, revision string) (string, error) {
	var p struct {
		SchemaVersion int      `json:"schema_version"`
		Profile       string   `json:"qualification_profile"`
		Revision      string   `json:"source_revision"`
		Inventory     string   `json:"inventory_sha256"`
		Binary        string   `json:"test_binary_sha256"`
		SID           string   `json:"effective_sid_sha256"`
		Standard      bool     `json:"actual_standard_user"`
		Elevated      bool     `json:"elevated"`
		Admin         bool     `json:"admin_member"`
		Tests         []string `json:"passed_tests"`
		Formats       int      `json:"real_formats"`
		Log           string   `json:"log_sha256"`
		Shared        bool     `json:"shared_implementation_attested"`
		MCP           string   `json:"mcp_server_execution"`
		Client        string   `json:"native_client"`
		Authority     string   `json:"publisher_authority"`
		Pack          string   `json:"runtime_pack_sha256"`
	}
	if e := releasecontract.DecodeStrictJSON(bytes.TrimPrefix(body, []byte{239, 187, 191}), &p); e != nil {
		return "", e
	}
	want := []string{"TestWindowsNativeConfinement", "TestWindowsNativeConfinementDeadline", "TestWindowsSharedImplementationIdentityAcrossEntryModules", "TestWindowsRealAuthenticatedDocling"}
	if p.SchemaVersion != 1 || p.Profile != "windows_docling_runtime_v1" || p.Revision != revision || !shaPattern.MatchString(p.Inventory) || !shaPattern.MatchString(p.Binary) || !shaPattern.MatchString(p.SID) || !p.Standard || p.Elevated || p.Admin || !reflect.DeepEqual(p.Tests, want) || p.Formats != 11 || p.Log != digest(log) || !p.Shared || p.MCP != "not_attested" || p.Client != "not_run" || p.Authority != "not_granted" || !shaPattern.MatchString(p.Pack) {
		return "", errors.New("exact successful ordinary-user full-profile proof required")
	}
	if bytes.Contains(log, []byte("--- FAIL:")) || bytes.Contains(log, []byte("--- SKIP:")) {
		return "", errors.New("native proof contains failed or skipped tests")
	}
	for _, name := range want {
		if !regexp.MustCompile("(?m)^--- PASS: " + regexp.QuoteMeta(name) + " ").Match(log) {
			return "", errors.New("required native parent did not pass")
		}
	}
	for _, name := range []string{"sample.docx", "literal.txt", "structured.md", "quoted.csv", "generic.json", "generic.xml", "page.html", "page.htm", "spine.epub", "legacy.xls", "sheets.xlsx", "validated_export", "installed_tamper", "missing_native_runtime"} {
		if !regexp.MustCompile("(?m)^    --- PASS: TestWindowsRealAuthenticatedDocling/" + regexp.QuoteMeta(name) + " ").Match(log) {
			return "", errors.New("required native format/control did not pass")
		}
	}
	matches := regexp.MustCompile("(?m)^    native_windows_test.go:[0-9]+: maestro_native_runtime_pack_sha256=([a-f0-9]{64})\r?$").FindAllSubmatch(log, -1)
	if len(matches) != 1 || string(matches[0][1]) != p.Pack {
		return "", errors.New("actual native pack observation differs")
	}
	return p.Pack, nil
}
func loadExport(o options) (receipt, error) {
	var r receipt
	b, e := read(filepath.Join(o.export, "export.json"), 64<<10)
	if e != nil {
		return r, e
	}
	if !shaPattern.MatchString(o.expected) || digest(b) != o.expected {
		return r, errors.New("reviewed export receipt pin differs")
	}
	sidecar, e := read(filepath.Join(o.export, "export.json.sha256"), 128)
	if e != nil {
		return r, e
	}
	if string(sidecar) != o.expected+"  export.json\n" {
		return r, errors.New("export receipt checksum sidecar differs")
	}
	if e = releasecontract.DecodeStrictJSON(b, &r); e != nil {
		return r, e
	}
	if r.SchemaVersion != 1 || r.Target != "windows/amd64" || r.SourceRevision != o.revision || r.SourceTree != o.tree || r.ProducerRunID != o.run || !revisionPattern.MatchString(o.revision) || !revisionPattern.MatchString(o.tree) || !runPattern.MatchString(o.run) || !shaPattern.MatchString(r.RegistrySHA256) || !shaPattern.MatchString(r.RuntimeSHA256) {
		return r, errors.New("export source, run, target or registry differs")
	}
	allowed := map[string]bool{"bootstrap.exe": true, "registry.json": true, "proof.json": true, "proof.log": true}
	previousVersion := ""
	for _, slot := range []string{"N", "N1"} {
		dir := filepath.Join(o.export, slot)
		if e = releasepack.VerifyCandidate(dir); e != nil {
			return r, e
		}
		m, e := manifest(dir)
		if e != nil {
			return r, e
		}
		if previousVersion != "" {
			if _, e := releasecontract.ParseVersionRange(">=" + previousVersion + " <" + m.Release); e != nil {
				return r, e
			}
		}
		previousVersion = m.Release
		if !m.Modern() || m.Server.ExecutionContract != releasecontract.SyncedSourceContract || len(m.Artifacts) != 3 {
			return r, errors.New("source-aware modern pair required")
		}
		count := 0
		for _, a := range m.Artifacts {
			if a.Kind == "runtime_pack" && a.Adapter == "docling" && a.OS == "windows" && a.Arch == "amd64" && a.SHA256 == r.RuntimeSHA256 {
				count++
			}
		}
		if count != 1 {
			return r, errors.New("actual proved runtime missing from release")
		}
		names := candidateMembers(m)
		if e = checkNames(dir, names); e != nil {
			return r, e
		}
		for _, name := range names {
			allowed[slot+"/"+name] = true
		}
	}
	if len(r.Files) != len(allowed) {
		return r, errors.New("export receipt membership differs")
	}
	seen := map[string]bool{}
	for _, m := range r.Files {
		if !allowed[m.Path] || seen[m.Path] || !shaPattern.MatchString(m.SHA256) || m.Size <= 0 || m.Size > 1<<30 {
			return r, errors.New("invalid closed export member")
		}
		seen[m.Path] = true
		b, e := read(filepath.Join(o.export, filepath.FromSlash(m.Path)), m.Size)
		if e != nil {
			return r, e
		}
		if int64(len(b)) != m.Size || digest(b) != m.SHA256 {
			return r, errors.New("export member changed")
		}
	}
	entries, e := os.ReadDir(o.export)
	if e != nil {
		return r, e
	}
	rootNames := []string{"N", "N1", "bootstrap.exe", "registry.json", "proof.json", "proof.log", "export.json", "export.json.sha256"}
	if len(entries) != len(rootNames) {
		return r, errors.New("unlisted export root input")
	}
	for _, v := range entries {
		found := false
		for _, n := range rootNames {
			if v.Name() == n {
				found = true
			}
		}
		if !found || v.Type()&os.ModeSymlink != 0 {
			return r, errors.New("foreign export root member")
		}
	}
	reg, e := read(filepath.Join(o.export, "registry.json"), 1<<20)
	if e != nil {
		return r, e
	}
	if digest(reg) != r.RegistrySHA256 {
		return r, errors.New("registry differs")
	}
	p, e := read(filepath.Join(o.export, "proof.json"), 64<<10)
	if e != nil {
		return r, e
	}
	log, e := read(filepath.Join(o.export, "proof.log"), 4<<20)
	if e != nil {
		return r, e
	}
	pack, e := verifyProof(p, log, o.revision)
	if e != nil {
		return r, e
	}
	if pack != r.RuntimeSHA256 {
		return r, errors.New("native proved runtime differs")
	}
	return r, nil
}
func publicNames(o options) (map[string]bool, error) {
	names := map[string]bool{"role.json": true, "role.json.sig": true, "current.json": true, "current.json.sig": true}
	for _, s := range []string{"N", "N1"} {
		m, e := manifest(filepath.Join(o.export, s))
		if e != nil {
			return nil, e
		}
		names[s+"/"+releaseverify.ManifestName] = true
		names[s+"/"+releaseverify.ManifestSignatureName] = true
		names[s+"/"+m.ReleaseNotes.Name] = true
		for _, a := range m.Artifacts {
			names[s+"/"+a.SignatureRef] = true
		}
	}
	return names, nil
}
func checkPublicFiles(files []publicFile, allowed map[string]bool) error {
	if len(files) != len(allowed) {
		return errors.New("public metadata requires exact closed members")
	}
	seen := map[string]bool{}
	total := 0
	for _, f := range files {
		total += len(f.Body)
		if !allowed[f.Path] || seen[f.Path] || f.Path != path.Clean(f.Path) || strings.ContainsAny(f.Path, "\\:") || len(f.Body) == 0 || total > maxPublicBytes {
			return errors.New("unexpected, duplicate or oversized public metadata member")
		}
		seen[f.Path] = true
	}
	return nil
}
func packMetadata(o options) error {
	r, e := loadExport(o)
	if e != nil {
		return e
	}
	names, e := publicNames(o)
	if e != nil {
		return e
	}
	p := publicMetadata{SchemaVersion: 1, ExportSHA256: o.expected}
	for _, s := range []struct{ slot, dir string }{{"N", o.signedN}, {"N1", o.signedN1}} {
		for name := range names {
			if !strings.HasPrefix(name, s.slot+"/") {
				continue
			}
			b, e := read(filepath.Join(s.dir, strings.TrimPrefix(name, s.slot+"/")), 16<<10)
			if e != nil {
				return e
			}
			p.Files = append(p.Files, publicFile{name, b})
		}
	}
	for name, from := range map[string]string{"role.json": o.role, "role.json.sig": o.signature, "current.json": filepath.Join(o.source, "current.json"), "current.json.sig": filepath.Join(o.source, "current.json.sig")} {
		b, e := read(from, 16<<10)
		if e != nil {
			return e
		}
		p.Files = append(p.Files, publicFile{name, b})
	}
	sort.Slice(p.Files, func(i, j int) bool { return p.Files[i].Path < p.Files[j].Path })
	if e = checkPublicFiles(p.Files, names); e != nil {
		return e
	}
	// Check actual authority and exact bodies before emitting transport for CI.
	if e = fresh(o.output); e != nil {
		return e
	}
	if _, e = reconstruct(o, r, p, filepath.Join(o.output, "verified")); e != nil {
		return e
	}
	b, e := json.Marshal(p)
	if e != nil {
		return e
	}
	if len(b) > maxPublicBytes {
		return errors.New("encoded public metadata exceeds dispatch bound")
	}
	encoded := []byte(base64.StdEncoding.EncodeToString(b))
	if e = write(filepath.Join(o.output, "public-metadata.base64"), encoded); e != nil {
		return e
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"public_metadata": filepath.Join(o.output, "public-metadata.base64"), "base64_bytes": len(encoded), "export_sha256": o.expected, "private_key_used": false})
}
func reconstruct(o options, r receipt, p publicMetadata, out string) (*installer.VerifiedExperimentalSeed, error) {
	names, e := publicNames(o)
	if e != nil {
		return nil, e
	}
	if p.SchemaVersion != 1 || p.ExportSHA256 != o.expected {
		return nil, errors.New("public metadata export identity differs")
	}
	if e = checkPublicFiles(p.Files, names); e != nil {
		return nil, e
	}
	if e = os.Mkdir(out, 0700); e != nil {
		return nil, e
	}
	for _, f := range p.Files {
		if e = write(filepath.Join(out, filepath.FromSlash(f.Path)), f.Body); e != nil {
			return nil, e
		}
	}
	reg, e := read(filepath.Join(o.export, "registry.json"), 1<<20)
	if e != nil {
		return nil, e
	}
	registry, e := releaseverify.ParseAuthorityRegistry(bytes.NewReader(reg), time.Now)
	if e != nil {
		return nil, e
	}
	rb, e := read(filepath.Join(out, "role.json"), 128<<10)
	if e != nil {
		return nil, e
	}
	rs, e := read(filepath.Join(out, "role.json.sig"), 64)
	if e != nil {
		return nil, e
	}
	role, e := installer.VerifyExperimentalSeedAdmission(rb, rs, reg, r.RegistrySHA256, time.Now)
	if e != nil {
		return nil, e
	}
	id := role.Identity()
	if id.TargetOS != "windows" || id.TargetArch != "amd64" {
		return nil, errors.New("Windows-specific finite role required")
	}
	if e = role.VerifyInstaller(filepath.Join(o.export, "bootstrap.exe"), id.InstallerVersion, "windows", "amd64"); e != nil {
		return nil, e
	}
	releases := map[string]releaseverify.VerifiedRelease{}
	for _, s := range []string{"N", "N1"} {
		unsigned, e := manifest(filepath.Join(o.export, s))
		if e != nil {
			return nil, e
		}
		signed, e := manifest(filepath.Join(out, s))
		if e != nil {
			return nil, e
		}
		compare := unsigned
		compare.Issuer = signed.Issuer
		compare.ReleaseNotes = signed.ReleaseNotes
		if !reflect.DeepEqual(compare, signed) || signed.ReleaseNotes.Name != unsigned.ReleaseNotes.Name {
			return nil, errors.New("signed metadata changes native candidate inputs")
		}
		for _, a := range unsigned.Artifacts {
			if e = copyOne(filepath.Join(o.export, s, a.Name), filepath.Join(out, s, a.Name), a.Size); e != nil {
				return nil, e
			}
		}
		verified, e := releaseverify.VerifyDirectory(filepath.Join(out, s), registry)
		if e != nil {
			return nil, e
		}
		if e = role.VerifyRelease(verified); e != nil {
			return nil, e
		}
		if _, exists := releases[verified.Manifest.Release]; exists {
			return nil, errors.New("release pair must be distinct")
		}
		releases[verified.Manifest.Release] = verified
	}
	body, e := read(filepath.Join(out, "current.json"), 64<<10)
	if e != nil {
		return nil, e
	}
	sig, e := read(filepath.Join(out, "current.json.sig"), 64)
	if e != nil {
		return nil, e
	}
	gen, e := releaseverify.VerifyGeneration(body, sig, registry)
	if e != nil {
		return nil, e
	}
	if e = gen.Generation.CheckTime(time.Now(), time.Time{}); e != nil {
		return nil, e
	}
	if e = role.VerifySource(gen.Generation.Source); e != nil {
		return nil, e
	}
	v, ok := releases[gen.Generation.Release]
	if !ok || v.ManifestSHA256 != gen.Generation.ManifestSHA256 || v.Manifest.Delivery.ClientInputsSHA256 != gen.Generation.ClientInputsSHA256 {
		return nil, errors.New("generation does not bind reconstructed native pair")
	}
	source := filepath.Join(out, "source")
	releaseDest := releaseprovider.SyncedReleaseDirectory(source, gen.Generation)
	entries, e := os.ReadDir(v.Directory)
	if e != nil {
		return nil, e
	}
	for _, f := range entries {
		if e = copyOne(filepath.Join(v.Directory, f.Name()), filepath.Join(releaseDest, f.Name()), 1<<30); e != nil {
			return nil, e
		}
	}
	for name, b := range map[string][]byte{"current.json": body, "current.json.sig": sig, "generations/" + gen.Generation.GenerationDirectory(gen.SHA256) + "/generation.json": body, "generations/" + gen.Generation.GenerationDirectory(gen.SHA256) + "/generation.json.sig": sig} {
		if e = write(filepath.Join(source, filepath.FromSlash(name)), b); e != nil {
			return nil, e
		}
	}
	observed, e := releaseprovider.ReadSyncedMetadata(source, id.Source, registry)
	if e != nil {
		return nil, e
	}
	if _, e = releaseprovider.ReadSyncedManifest(source, observed, registry); e != nil {
		return nil, e
	}
	return role, nil
}
func assembleNative(o options) error {
	if e := native(); e != nil {
		return e
	}
	if e := checkNativeSource(o); e != nil {
		return e
	}
	r, e := loadExport(o)
	if e != nil {
		return e
	}
	encoded, e := read(o.metadata, maxEncodedPublicBytes)
	if e != nil {
		return e
	}
	if len(encoded) > maxEncodedPublicBytes {
		return errors.New("metadata transport exceeds exact dispatch bound")
	}
	b, e := base64.StdEncoding.Strict().DecodeString(string(encoded))
	if e != nil {
		return e
	}
	if len(b) > maxPublicBytes {
		return errors.New("metadata exceeds bound")
	}
	var p publicMetadata
	if e = releasecontract.DecodeStrictJSON(b, &p); e != nil {
		return e
	}
	if e = fresh(o.output); e != nil {
		return e
	}
	verified := filepath.Join(o.output, "verified")
	role, e := reconstruct(o, r, p, verified)
	if e != nil {
		return e
	}
	id := role.Identity()
	kit := filepath.Join(o.output, "Maestro-MCP-Windows")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "go", "run", "./dev/pilot/installed-seed-factory", "--out", kit, "--installer", filepath.Join(o.export, "bootstrap.exe"), "--role", filepath.Join(verified, "role.json"), "--role-signature", filepath.Join(verified, "role.json.sig"), "--registry", filepath.Join(o.export, "registry.json"), "--registry-sha256", r.RegistrySHA256, "--initial-release", filepath.Join(verified, "N"), "--source", filepath.Join(verified, "source"), "--templates", filepath.Join(o.root, "dev/pilot/templates/installed-seed"))
	command.Dir = o.root
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if e = command.Run(); e != nil {
		return fmt.Errorf("original native factory refused; staged inputs preserved: %w", e)
	}
	if e = role.Revalidate(); e != nil {
		return e
	}
	want, e := kitFiles(o, verified, kit, role)
	if e != nil {
		return e
	}
	archive := kit + ".zip"
	if e = zipKit(kit, archive, want); e != nil {
		return e
	}
	z, e := read(archive, 1<<30)
	if e != nil {
		return e
	}
	if e = write(archive+".sha256", []byte(digest(z)+"  "+filepath.Base(archive)+"\n")); e != nil {
		return e
	}
	text := "# Maestro MCP — Windows experimental pilot\n\nDownload the ZIP and extract the whole folder. Open Maestro.cmd and follow the initial preparation. Use option 1 (Ensaio completo Code), then 2 (Retomar Desktop); option 7 saves the participant report. Option 8 checks the shared-folder update. Keep the shared source synced locally and your private brain in Developer. No Go, Git, Python, WSL or administrator install is required.\n\nThis kit is engineering-only and expires with its finite admission. Actual Claude Code/Desktop login, plugin loading, hooks, agent execution and OneDrive update acceptance remain participant-local checks.\n"
	if e = write(filepath.Join(o.output, "LEIA-ME-WINDOWS.md"), []byte(text)); e != nil {
		return e
	}
	result := map[string]string{"zip": filepath.Base(archive), "sha256": digest(z), "export_sha256": o.expected, "source_revision": r.SourceRevision, "source_tree": r.SourceTree, "producer_run_id": r.ProducerRunID, "admission_sha256": id.AdmissionSHA256, "runtime_pack_sha256": r.RuntimeSHA256, "evidence": "engineering_assembled_native_factory", "native_client": "not_run"}
	identity, e := jsonBody(result)
	if e != nil {
		return e
	}
	if e = write(filepath.Join(o.output, "BUILD-IDENTITY.json"), identity); e != nil {
		return e
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}

func kitFiles(o options, verified, kit string, role *installer.VerifiedExperimentalSeed) (map[string]string, error) {
	want := map[string]string{}
	add := func(name, from string) error {
		b, e := read(from, 1<<30)
		if e == nil {
			want[name] = digest(b)
		}
		return e
	}
	id := role.Identity()
	for name, from := range map[string]string{
		"bootstrap/maestro-install_" + id.InstallerVersion + "_windows_amd64.exe": filepath.Join(o.export, "bootstrap.exe"),
		"bootstrap/trust/experimental-installed-seed.json":                        filepath.Join(verified, "role.json"),
		"bootstrap/trust/experimental-installed-seed.json.sig":                    filepath.Join(verified, "role.json.sig"),
		"bootstrap/trust/release-authority-registry.json":                         filepath.Join(o.export, "registry.json"),
	} {
		if e := add(name, from); e != nil {
			return nil, e
		}
	}
	document, e := pilotdocument.Bytes()
	if e != nil {
		return nil, e
	}
	want["test-documents/"+pilotdocument.Name] = digest(document)
	release := func(dir, prefix string) error {
		m, e := manifest(dir)
		if e != nil {
			return e
		}
		names := candidateMembers(m)
		names = append(names, releaseverify.ManifestSignatureName)
		for _, a := range m.Artifacts {
			names = append(names, a.SignatureRef)
		}
		for _, name := range names {
			if e = add(prefix+"/"+name, filepath.Join(dir, name)); e != nil {
				return e
			}
		}
		return nil
	}
	if e = release(filepath.Join(verified, "N"), "initial-release"); e != nil {
		return nil, e
	}
	body, e := read(filepath.Join(verified, "source", "current.json"), 64<<10)
	if e != nil {
		return nil, e
	}
	g, e := releasecontract.ParseGeneration(body)
	if e != nil {
		return nil, e
	}
	prefix := "releases/" + g.Release + "/windows-amd64"
	if e = release(filepath.Join(verified, "source", filepath.FromSlash(prefix)), "source/"+prefix); e != nil {
		return nil, e
	}
	for _, name := range []string{"current.json", "current.json.sig", "generations/" + g.GenerationDirectory(digest(body)) + "/generation.json", "generations/" + g.GenerationDirectory(digest(body)) + "/generation.json.sig"} {
		if e = add("source/"+name, filepath.Join(verified, "source", filepath.FromSlash(name))); e != nil {
			return nil, e
		}
	}
	distribution, e := read(filepath.Join(kit, "DISTRIBUTION.json"), 64<<10)
	if e != nil {
		return nil, e
	}
	want["DISTRIBUTION.json"] = digest(distribution)
	for _, name := range []string{"LEIA-ME.md", "Maestro.cmd", "Maestro.ps1"} {
		b, e := read(filepath.Join(o.root, "dev/pilot/templates/installed-seed", name), 256<<10)
		if e != nil {
			return nil, e
		}
		b = bytes.ReplaceAll(b, []byte("@@DISTRIBUTION_SHA256@@"), []byte(digest(distribution)))
		want[name] = digest(b)
	}
	return want, nil
}
func zipKit(root, destination string, want map[string]string) error {
	seen := map[string]bool{}
	e := filepath.WalkDir(root, func(p string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		rel, e := filepath.Rel(root, p)
		if e != nil {
			return e
		}
		name := filepath.ToSlash(rel)
		if _, ok := want[name]; !ok || seen[name] || d.Type()&os.ModeSymlink != 0 {
			return errors.New("unlisted or unsafe factory output; ZIP was not published")
		}
		seen[name] = true
		return nil
	})
	if e != nil {
		return e
	}
	if len(seen) != len(want) {
		return errors.New("factory output misses expected member")
	}
	f, e := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	z := zip.NewWriter(f)
	names := make([]string, 0, len(want))
	for name := range want {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		b, err := read(filepath.Join(root, filepath.FromSlash(name)), 1<<30)
		if err != nil {
			e = err
			break
		}
		if digest(b) != want[name] {
			e = errors.New("factory bytes changed before ZIP publication")
			break
		}
		w, err := z.Create(filepath.Base(root) + "/" + name)
		if err != nil {
			e = err
			break
		}
		if _, err = w.Write(b); err != nil {
			e = err
			break
		}
	}
	ze := z.Close()
	fe := f.Close()
	if e != nil {
		return e
	}
	if ze != nil {
		return ze
	}
	return fe
}
