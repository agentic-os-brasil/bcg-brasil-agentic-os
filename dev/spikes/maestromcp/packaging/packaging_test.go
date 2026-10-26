package packaging

import (
	"archive/zip"
	"bytes"
	"debug/macho"
	"encoding/binary"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	techcoreskills "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/tech-core/skills"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/dev/spikes/maestromcp/spike"
)

// Parser fixtures are never executed and do not qualify either native host.
func nativeFixture(platform string) []byte {
	if platform == "darwin" {
		var b bytes.Buffer
		for _, v := range []uint32{macho.Magic64, uint32(macho.CpuArm64), 0, uint32(macho.TypeExec), 0, 0, 0, 0} {
			_ = binary.Write(&b, binary.LittleEndian, v)
		}
		return b.Bytes()
	}
	b := make([]byte, 0x80+24)
	copy(b, "MZ")
	binary.LittleEndian.PutUint32(b[0x3c:], 0x80)
	copy(b[0x80:], "PE\x00\x00")
	binary.LittleEndian.PutUint16(b[0x84:], 0x8664)
	binary.LittleEndian.PutUint16(b[0x96:], 0x0002)
	return b
}
func fixture(t *testing.T, platform string) Options {
	t.Helper()
	base := t.TempDir()
	root := filepath.Join(base, "state")
	if _, err := spike.Seed(root); err != nil {
		t.Fatal(err)
	}
	server := filepath.Join(base, "server")
	if err := os.WriteFile(server, nativeFixture(platform), 0700); err != nil {
		t.Fatal(err)
	}
	return Options{Destination: filepath.Join(base, "plugin"), Server: server, StateRoot: root, Workspace: "case-a", Version: "0.0.3", Name: "maestro-team-pilot"}
}
func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	got := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		b, err := os.ReadFile(p)
		got[p] = string(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return got
}
func TestPluginNamespacedAndStatePreserved(t *testing.T) {
	opts := fixture(t, "darwin")
	before := snapshot(t, opts.StateRoot)
	if err := BuildPlugin(opts); err != nil {
		t.Fatal(err)
	}
	skill, err := os.ReadFile(filepath.Join(opts.Destination, "skills", "operator", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(skill), "/maestro-team-pilot:resume") || strings.Contains(string(skill), "/maestro-architecture-spike:resume") {
		t.Fatal("wrong command namespace")
	}
	var manifest map[string]any
	b, _ := os.ReadFile(filepath.Join(opts.Destination, ".claude-plugin", "plugin.json"))
	if err := json.Unmarshal(b, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest["name"] != opts.Name || manifest["version"] != opts.Version {
		t.Fatal("wrong identity")
	}
	if !reflect.DeepEqual(before, snapshot(t, opts.StateRoot)) {
		t.Fatal("state changed")
	}
	if err := BuildPlugin(opts); err == nil {
		t.Fatal("existing destination accepted")
	}
}

func TestPluginIncludesPocockMethodsWithResourcesAndNativeReferences(t *testing.T) {
	for _, name := range []string{"maestro-team-pilot", "maestro-architecture-spike"} {
		t.Run(name, func(t *testing.T) {
			opts := fixture(t, "darwin")
			opts.Name = name
			before := snapshot(t, opts.StateRoot)
			if err := BuildPlugin(opts); err != nil {
				t.Fatal(err)
			}
			catalog, err := techcoreskills.Catalog()
			if err != nil {
				t.Fatal(err)
			}
			methods, manual := 0, 0
			for _, skill := range catalog.Skills {
				if !strings.HasPrefix(skill.ID, "pocock-") {
					continue
				}
				methods++
				resources, err := techcoreskills.Files(skill.ID)
				if err != nil {
					t.Fatal(err)
				}
				for relative, original := range resources {
					path := filepath.Join(opts.Destination, "skills", skill.ID, filepath.FromSlash(relative))
					got, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					info, err := os.Stat(path)
					if err != nil || info.Mode().Perm()&0111 != 0 {
						t.Fatalf("method resource must be inert: %s", path)
					}
					if relative == "SKILL.md" {
						frontmatter := strings.SplitN(string(original), "\n---\n", 2)[0] + "\n---\n"
						if !bytes.HasPrefix(got, []byte(frontmatter)) || !bytes.Contains(got, []byte("/"+name+":interaction-profile")) {
							t.Fatalf("frontmatter or dependency reference lost: %s", skill.ID)
						}
						if skill.UserInvokedOnly {
							manual++
							if !bytes.Contains(got, []byte("disable-model-invocation: true")) {
								t.Fatalf("manual-only policy lost: %s", skill.ID)
							}
						}
					}
					if strings.HasSuffix(relative, ".md") {
						canonical := strings.ReplaceAll(string(got), name+":", "")
						if canonical != strings.ReplaceAll(string(original), "$interaction-profile", "/interaction-profile") {
							t.Fatalf("Markdown changed beyond invocation projection: %s/%s", skill.ID, relative)
						}
					} else if !bytes.Equal(got, original) {
						t.Fatalf("non-Markdown resource changed: %s/%s", skill.ID, relative)
					}
				}
				if len(resources["LICENSE"]) == 0 {
					t.Fatalf("missing upstream license: %s", skill.ID)
				}
			}
			if methods != 27 || manual != 16 {
				t.Fatalf("packaged methods=%d manual-only=%d; want 27 and 16", methods, manual)
			}
			read := func(id, relative string) string {
				t.Helper()
				body, err := os.ReadFile(filepath.Join(opts.Destination, "skills", id, relative))
				if err != nil {
					t.Fatal(err)
				}
				return string(body)
			}
			if !strings.Contains(read("pocock-grill-with-docs", "SKILL.md"), `for "`+name+`:pocock-grilling" and "`+name+`:pocock-domain-modeling"`) {
				t.Fatal("Skill tool cross-method calls are not plugin namespaced")
			}
			if !strings.Contains(read("pocock-ask-matt", "SKILL.md"), "/"+name+":pocock-to-spec") || !strings.Contains(read("pocock-improve-codebase-architecture", "HTML-REPORT.md"), "/"+name+":pocock-codebase-design") {
				t.Fatal("slash invocation in method or linked resource is not plugin namespaced")
			}
			read("interaction-profile", "SKILL.md")
			provenanceBody, err := os.ReadFile(filepath.Join(opts.Destination, "pocock-upstream.json"))
			if err != nil {
				t.Fatal(err)
			}
			var provenance struct {
				Canonical  json.RawMessage `json:"canonical"`
				Projection string          `json:"projection"`
				Namespace  string          `json:"namespace"`
				HashScope  string          `json:"hash_scope"`
			}
			if err := json.Unmarshal(provenanceBody, &provenance); err != nil {
				t.Fatal(err)
			}
			var canonical struct {
				Version  string `json:"upstream_version"`
				Revision string `json:"upstream_revision"`
			}
			if err := json.Unmarshal(provenance.Canonical, &canonical); err != nil {
				t.Fatal(err)
			}
			if canonical.Version != "1.3.1" || canonical.Revision != "49dd158d1076134a641b33efb035946536778336" || provenance.Projection != "markdown_invocation_namespace" || provenance.Namespace != name || provenance.HashScope != "canonical_product_source_before_projection" {
				t.Fatal("upstream pin or plugin projection provenance lost")
			}
			var expected, actual bytes.Buffer
			if json.Compact(&expected, techcoreskills.Provenance()) != nil || json.Compact(&actual, provenance.Canonical) != nil || !bytes.Equal(expected.Bytes(), actual.Bytes()) {
				t.Fatal("canonical upstream provenance changed")
			}
			if !reflect.DeepEqual(before, snapshot(t, opts.StateRoot)) {
				t.Fatal("external state changed while packaging methods")
			}
		})
	}
}
func TestExtensionTargetAndExactBinary(t *testing.T) {
	for _, platform := range []string{"darwin", "win32"} {
		t.Run(platform, func(t *testing.T) {
			opts := fixture(t, platform)
			opts.Destination += ".mcpb"
			before := snapshot(t, opts.StateRoot)
			if err := BuildExtension(opts); err != nil {
				t.Fatal(err)
			}
			r, err := zip.OpenReader(opts.Destination)
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			if len(r.File) != 2 {
				t.Fatal("unexpected files")
			}
			entries := map[string][]byte{}
			for _, f := range r.File {
				in, err := f.Open()
				if err != nil {
					t.Fatal(err)
				}
				entries[f.Name], err = io.ReadAll(in)
				in.Close()
				if err != nil {
					t.Fatal(err)
				}
			}
			path := "server/maestro-mcp-spike"
			if platform == "win32" {
				path += ".exe"
			}
			if !bytes.Equal(entries[path], nativeFixture(platform)) {
				t.Fatal("binary differs")
			}
			var m struct {
				Name          string `json:"name"`
				Compatibility struct {
					Platforms []string `json:"platforms"`
				} `json:"compatibility"`
				Server struct {
					Entry string `json:"entry_point"`
				} `json:"server"`
			}
			if err := json.Unmarshal(entries["manifest.json"], &m); err != nil {
				t.Fatal(err)
			}
			if m.Name != opts.Name || !reflect.DeepEqual(m.Compatibility.Platforms, []string{platform}) || m.Server.Entry != path {
				t.Fatalf("bad target %+v", m)
			}
			if !reflect.DeepEqual(before, snapshot(t, opts.StateRoot)) {
				t.Fatal("state changed")
			}
			if err := BuildExtension(opts); err == nil {
				t.Fatal("existing artifact accepted")
			}
		})
	}
}
func TestBuildRejectsInvalidBeforeOutput(t *testing.T) {
	for _, name := range []string{"version", "name", "binary", "fixture", "symlink"} {
		t.Run(name, func(t *testing.T) {
			opts := fixture(t, "darwin")
			switch name {
			case "version":
				opts.Version = "01.2.3"
			case "name":
				opts.Name = "../bad"
			case "binary":
				os.WriteFile(opts.Server, []byte("script"), 0700)
			case "fixture":
				opts.StateRoot = filepath.Dir(opts.StateRoot)
			case "symlink":
				p := opts.Server + "-link"
				if err := os.Symlink(opts.Server, p); err != nil {
					t.Fatal(err)
				}
				opts.Server = p
			}
			if err := BuildPlugin(opts); err == nil {
				t.Fatal("invalid input accepted")
			}
			if _, err := os.Lstat(opts.Destination); !os.IsNotExist(err) {
				t.Fatal("output created")
			}
			opts.Destination += ".mcpb"
			if err := BuildExtension(opts); err == nil {
				t.Fatal("invalid extension input accepted")
			}
			if _, err := os.Lstat(opts.Destination); !os.IsNotExist(err) {
				t.Fatal("artifact created")
			}
		})
	}
}

func TestNativeTargetRejectsLibrariesAndUnsupportedArchitectures(t *testing.T) {
	for _, name := range []string{"windows-dll", "windows-x86", "macos-dylib", "macos-x86"} {
		t.Run(name, func(t *testing.T) {
			platform := "win32"
			if strings.HasPrefix(name, "macos") {
				platform = "darwin"
			}
			o := fixture(t, platform)
			body := nativeFixture(platform)
			switch name {
			case "windows-dll":
				binary.LittleEndian.PutUint16(body[0x96:], 0x2002)
			case "windows-x86":
				binary.LittleEndian.PutUint16(body[0x84:], 0x014c)
			case "macos-dylib":
				binary.LittleEndian.PutUint32(body[12:], uint32(macho.TypeDylib))
			case "macos-x86":
				binary.LittleEndian.PutUint32(body[4:], uint32(macho.Cpu386))
			}
			if err := os.WriteFile(o.Server, body, 0700); err != nil {
				t.Fatal(err)
			}
			if err := BuildPlugin(o); err == nil {
				t.Fatal("unsupported native input accepted")
			}
			o.Destination += ".mcpb"
			if err := BuildExtension(o); err == nil {
				t.Fatal("unsupported extension target accepted")
			}
		})
	}
}
