package execution

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateSchemaFilePublicReadPreservesBytesAndModes(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "schemas", "execution-state.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	original := filepath.Join(root, "published.json")
	if err = os.WriteFile(original, body, 0644); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "published-hardlink.json")
	if err = os.Link(original, alias); err != nil {
		t.Fatal("required public hardlink fixture unavailable", err)
	}
	before, err := os.Stat(original)
	if err != nil {
		t.Fatal(err)
	}
	relative := filepath.Join("..", "..", "schemas", "execution-state.schema.json")
	for _, path := range []string{original, alias, relative} {
		if err = ValidateSchemaFile(path); err != nil {
			t.Fatalf("read-only public schema %s: %v", path, err)
		}
	}
	for _, path := range []string{original, alias} {
		after, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, body) || !os.SameFile(before, after) || before.Mode() != after.Mode() || !before.ModTime().Equal(after.ModTime()) {
			t.Fatalf("public schema changed: %s %v", path, err)
		}
	}
}

func TestValidateSchemaFileRejectsUnsafePublicInput(t *testing.T) {
	published, err := os.ReadFile(filepath.Join("..", "..", "schemas", "execution-state.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"trailing-value", "trailing-invalid", "malformed", "oversized", "directory", "leaf-alias", "ancestor-alias", "schema-identity"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "schema.json")
			body := append([]byte(nil), published...)
			switch kind {
			case "trailing-value":
				body = append(body, []byte(" {}")...)
			case "trailing-invalid":
				body = append(body, '!')
			case "malformed":
				body = []byte("{")
			case "oversized":
				body = append(body, bytes.Repeat([]byte(" "), maximumLedgerJSONBytes)...)
			case "schema-identity":
				body = bytes.Replace(body, []byte("urn:bcg-brasil-agentic-os:schema:execution-state:v1"), []byte("urn:unexpected"), 1)
			case "directory":
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			case "leaf-alias", "ancestor-alias":
				target := filepath.Join(root, "target")
				if err := os.Mkdir(target, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(target, "schema.json"), published, 0644); err != nil {
					t.Fatal(err)
				}
				if kind == "leaf-alias" {
					schemaLeafAlias(t, path, target)
				} else {
					confinementDirectoryAlias(t, path, target)
					path = filepath.Join(path, "schema.json")
				}
			}
			if kind != "directory" && kind != "leaf-alias" && kind != "ancestor-alias" {
				if err := os.WriteFile(path, body, 0644); err != nil {
					t.Fatal(err)
				}
			}
			if err := ValidateSchemaFile(path); err == nil {
				t.Fatal("accepted unsafe public schema")
			}
			if kind == "leaf-alias" || kind == "ancestor-alias" {
				got, err := os.ReadFile(filepath.Join(root, "target", "schema.json"))
				if err != nil || !bytes.Equal(got, published) {
					t.Fatal("alias target bytes changed", err)
				}
			}
		})
	}
}
