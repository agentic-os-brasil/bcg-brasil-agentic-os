package atlasops

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type atlasTextSnapshot struct {
	Mode os.FileMode
	Body string
}

func snapshotAtlasText(t *testing.T, root string) map[string]atlasTextSnapshot {
	t.Helper()
	result := map[string]atlasTextSnapshot{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		value := atlasTextSnapshot{Mode: info.Mode()}
		if !entry.IsDir() {
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			value.Body = string(body)
		}
		result[relative] = value
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func seedAtlasText(t *testing.T) *Engine {
	t.Helper()
	engine := testEngine(t)
	if _, err := engine.CreatePage(CreatePageRequest{Page: "knowledge/notes.md", Body: "# Notes\n\n- **Status:** draft\n\n## Evidence\n", Provenance: attended("seed-text")}); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.CreateGrant(GrantRequest{Ritual: "text-fixture", RitualVersion: "1", Segment: "knowledge", Operations: []string{"append-entry"}, Cadence: "manual", Reader: ReaderOwnerSession}); err != nil {
		t.Fatal(err)
	}
	return engine
}

func TestAtlasPublicRejectsMalformedUTF8BeforeEffects(t *testing.T) {
	cases := []struct {
		name string
		call func(*Engine, string) error
	}{
		{"create body", func(e *Engine, bad string) error {
			_, err := e.CreatePage(CreatePageRequest{Page: "knowledge/new.md", Body: "# " + bad, Provenance: attended("same-create")})
			return err
		}},
		{"append entry", func(e *Engine, bad string) error {
			_, err := e.AppendEntry(AppendEntryRequest{Page: "knowledge/notes.md", Section: "## Evidence", Entry: "- " + bad, Provenance: attended("same-append")})
			return err
		}},
		{"field value", func(e *Engine, bad string) error {
			_, err := e.SetField(SetFieldRequest{Page: "knowledge/notes.md", Field: "Status", Value: bad, Provenance: attended("same-field")})
			return err
		}},
		{"link label", func(e *Engine, bad string) error {
			_, err := e.Link(LinkRequest{Page: "knowledge/notes.md", Section: "## Evidence", Target: "knowledge/target.md", Label: bad, Provenance: attended("same-link")})
			return err
		}},
		{"page identity", func(e *Engine, bad string) error {
			_, err := e.CreatePage(CreatePageRequest{Page: "knowledge/" + bad + ".md", Body: "# Valid\n", Provenance: attended("same-page")})
			return err
		}},
		{"provenance", func(e *Engine, bad string) error {
			p := attended("same-provenance")
			p.SessionID = bad
			_, err := e.CreatePage(CreatePageRequest{Page: "knowledge/new.md", Body: "# Valid\n", Provenance: p})
			return err
		}},
		{"grant ritual", func(e *Engine, bad string) error {
			_, err := e.CreateGrant(GrantRequest{Ritual: bad, RitualVersion: "1", Segment: "knowledge", Operations: []string{"append-entry"}, Cadence: "manual", Reader: ReaderOwnerSession})
			return err
		}},
		{"collect purpose", func(e *Engine, bad string) error {
			_, err := e.Collect(CollectRequest{Purpose: bad, Reader: ReaderOwnerSession, Pages: []string{"knowledge/notes.md"}})
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine := seedAtlasText(t)
			before := snapshotAtlasText(t, engine.Root())
			for _, bad := range []string{string([]byte{0xff}), string([]byte{0xfe})} {
				if err := tc.call(engine, bad); err == nil {
					t.Errorf("malformed %s accepted", tc.name)
				}
				if after := snapshotAtlasText(t, engine.Root()); !reflect.DeepEqual(before, after) {
					t.Errorf("malformed %s changed page, journal or grant state", tc.name)
				}
			}
		})
	}
}

func TestAtlasMalformedExistingUTF8IsRefusedWithoutRepair(t *testing.T) {
	engine := seedAtlasText(t)
	page := filepath.Join(engine.Root(), "knowledge/notes.md")
	if err := os.WriteFile(page, []byte("# Notes\n\n- **Status:** draft\n\n## Evidence\n"+string([]byte{0xff})+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	before := snapshotAtlasText(t, engine.Root())
	calls := []func() error{
		func() error {
			_, err := engine.Collect(CollectRequest{Purpose: "read existing page", Reader: ReaderOwnerSession, Pages: []string{"knowledge/notes.md"}})
			return err
		},
		func() error {
			_, err := engine.AppendEntry(AppendEntryRequest{Page: "knowledge/notes.md", Section: "## Evidence", Entry: "- valid entry", Provenance: attended("existing-append")})
			return err
		},
		func() error {
			_, err := engine.SetField(SetFieldRequest{Page: "knowledge/notes.md", Field: "Status", Value: "reviewed", Provenance: attended("existing-field")})
			return err
		},
		func() error {
			_, err := engine.Link(LinkRequest{Page: "knowledge/notes.md", Section: "## Evidence", Target: "knowledge/target.md", Label: "valid label", Provenance: attended("existing-link")})
			return err
		},
		func() error {
			_, err := engine.CreatePage(CreatePageRequest{Page: "knowledge/notes.md", Body: "# Valid replacement\n", Provenance: attended("existing-create")})
			return err
		},
	}
	for i, call := range calls {
		if err := call(); err == nil {
			t.Errorf("malformed existing page accepted by operation %d", i)
		}
		if !reflect.DeepEqual(before, snapshotAtlasText(t, engine.Root())) {
			t.Errorf("operation %d changed malformed original or controls", i)
		}
	}
}

func TestAtlasMalformedGrantJSONIsRefusedWithoutRepair(t *testing.T) {
	engine := seedAtlasText(t)
	path := engine.grantsPath()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	body = []byte(strings.Replace(string(body), "text-fixture", string([]byte{0xff}), 1))
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	before := snapshotAtlasText(t, engine.Root())
	if _, err := engine.Grants(); err == nil {
		t.Error("malformed grant JSON was normalized into trusted state")
	}
	if !reflect.DeepEqual(before, snapshotAtlasText(t, engine.Root())) {
		t.Fatal("malformed grant state changed")
	}
}
