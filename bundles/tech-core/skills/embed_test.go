package techcoreskills

import "testing"

func TestFilesIncludesPocockRelativeGuidesAndLicense(t *testing.T) {
	files, err := Files("pocock-tdd")
	if err != nil {
		t.Fatal(err)
	}
	for _, relative := range []string{"SKILL.md", "tests.md", "mocking.md", "LICENSE", "agents/openai.yaml"} {
		if len(files[relative]) == 0 {
			t.Fatalf("embedded Pocock method missing %s", relative)
		}
	}
	for _, id := range []string{"../catalog.json", "catalog.json", "unknown-method"} {
		if _, err := Files(id); err == nil {
			t.Fatalf("accepted factory path %q", id)
		}
	}
}
