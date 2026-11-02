package claudeagents_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	baseagents "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/agents"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/claudeagents"
)

func TestNativeSpecialistInventoryAndFrontmatter(t *testing.T) {
	workspace := t.TempDir()
	status, err := claudeagents.Install(workspace)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"case-agent", "client-account-agent", "darwin", "gamma-guardian", "pa-expert", "yoda"}
	if !reflect.DeepEqual(status.Agents, want) {
		t.Errorf("specialists = %v; want %v", status.Agents, want)
	}
	for _, id := range want {
		body, err := os.ReadFile(filepath.Join(workspace, ".claude", "agents", id+".md"))
		if err != nil {
			t.Errorf("%s: %v", id, err)
			continue
		}
		if !strings.HasPrefix(string(body), "---\nname: "+id+"\n") {
			t.Errorf("%s: native frontmatter must start at byte zero", id)
		}
		tools := "tools: []\n"
		if id == "case-agent" {
			tools = "tools: Read, Write, Edit, Glob, Grep\n"
		}
		if !strings.Contains(string(body), tools) {
			t.Errorf("%s: wrong tool contract", id)
		}
	}
	if _, err := os.Stat(filepath.Join(workspace, ".claude", "agents", "maestro.md")); !os.IsNotExist(err) {
		t.Fatal("hub projected as a specialist")
	}
}

func TestAllSevenCanonicalDefinitionsAvailable(t *testing.T) {
	for _, id := range []string{"case-agent", "client-account-agent", "darwin", "gamma-guardian", "maestro", "pa-expert", "yoda"} {
		body, err := baseagents.Definition(id)
		if err != nil || len(body) == 0 {
			t.Errorf("canonical definition %s unavailable: %v", id, err)
		}
	}
}

func TestPureProjectionNamesAndInvocationReferences(t *testing.T) {
	workspace, err := claudeagents.Project("")
	if err != nil {
		t.Fatal(err)
	}
	plugin, err := claudeagents.Project("maestro-product")
	if err != nil {
		t.Fatal(err)
	}
	again, err := claudeagents.Project("maestro-product")
	if err != nil || !reflect.DeepEqual(plugin, again) {
		t.Fatal("projection is nondeterministic")
	}
	if len(plugin) != 6 || len(workspace) != 6 {
		t.Fatal("wrong specialist inventory")
	}
	installed := t.TempDir()
	if _, err := claudeagents.Install(installed); err != nil {
		t.Fatal(err)
	}
	for i, id := range []string{"case-agent", "client-account-agent", "darwin", "gamma-guardian", "pa-expert", "yoda"} {
		p := plugin[i]
		if p.ID != id || p.Name != "maestro-product:"+id || p.Path != "agents/"+id+".md" {
			t.Fatalf("wrong native identity: %+v", p)
		}
		// The host namespaces plugin invocation names. The actual file and YAML
		// identity must stay bare so the host cannot qualify the name twice.
		if !strings.HasPrefix(string(p.Body), "---\nname: "+id+"\n") {
			t.Fatalf("plugin host received a qualified or invalid frontmatter name: %s", id)
		}
		if workspace[i].Name != id || workspace[i].ID != id || string(workspace[i].Body) != string(p.Body) {
			t.Fatalf("namespace changed identity contract %s", id)
		}
		body, err := os.ReadFile(filepath.Join(installed, ".claude", p.Path))
		if err != nil || string(body) != string(workspace[i].Body) {
			t.Fatalf("workspace did not use shared projector: %s %v", id, err)
		}
		canonical, err := baseagents.Definition(id)
		if err != nil || !strings.HasSuffix(string(p.Body), string(canonical)) {
			t.Fatalf("canonical definition changed: %s", id)
		}
		if !strings.Contains(string(p.Body), "native_advisory is unavailable") || strings.Contains(string(p.Body), "{{") {
			t.Fatalf("profile/defaults invalid: %s", id)
		}
	}
	gamma := string(plugin[3].Body)
	for _, clause := range []string{"tools: []", "tool-free bounded packet analysis only", "inspection grants and longitudinal custody are unavailable", "UNAVAILABLE/BLOCKED", "bounded_quality_packet"} {
		if !strings.Contains(gamma, clause) {
			t.Errorf("Gamma missing %q", clause)
		}
	}
	plugin[0].Body[0] = 'X'
	if workspace[0].Body[0] != '-' || again[0].Body[0] != '-' {
		t.Fatal("caller mutated shared projection")
	}
	for _, namespace := range []string{"other", "maestro-product:spoof", " maestro-product"} {
		if _, err := claudeagents.Project(namespace); err == nil {
			t.Errorf("unadmitted namespace %q", namespace)
		}
	}
	input := []byte("Agent(subagent_type=\"yoda\", prompt=packet)\nAgent tool, `subagent_type: \"gamma-guardian\"`\nAgent\ntool, `subagent_type: \"yoda\"`\n{\"subagent_type\":\"yoda\",\"id\":\"gamma-guardian\"}\n.claude/agents/yoda.md\nbundles/base/skills/yoda/SKILL.md\nAgent(subagent_type=\"external:yoda\")\n")
	want := "Agent(subagent_type=\"maestro-product:yoda\", prompt=packet)\nAgent tool, `subagent_type: \"maestro-product:gamma-guardian\"`\nAgent\ntool, `subagent_type: \"maestro-product:yoda\"`\n{\"subagent_type\":\"yoda\",\"id\":\"gamma-guardian\"}\n.claude/agents/yoda.md\nbundles/base/skills/yoda/SKILL.md\nAgent(subagent_type=\"external:yoda\")\n"
	projected, err := claudeagents.ProjectAgentInvocations(input, "maestro-product")
	if err != nil || string(projected) != want {
		t.Fatalf("invocation projection = %s; err=%v", projected, err)
	}
	retry, err := claudeagents.ProjectAgentInvocations(projected, "maestro-product")
	if err != nil || string(retry) != want {
		t.Fatal("invocation projection not idempotent")
	}
	if _, err := claudeagents.ProjectAgentInvocations([]byte(`Agent(subagent_type="maestro-product:unknown")`), "maestro-product"); err == nil {
		t.Fatal("unknown own specialist projected")
	}
}

func TestExactNativeNamespaceAndToolPolicies(t *testing.T) {
	workspace := t.TempDir()
	for _, id := range []string{"case-agent", "client-account-agent", "darwin", "gamma-guardian", "pa-expert", "yoda"} {
		for _, name := range []string{id, "maestro-product:" + id} {
			canonical, ok := claudeagents.CanonicalAgentID(name)
			if canonical != id || !ok || !claudeagents.Managed(name) {
				t.Fatalf("valid specialist rejected %q", name)
			}
			reason, recognized := claudeagents.GuardTool(name, "Bash", []byte(`{"command":"pwd"}`), workspace, workspace)
			if !recognized || reason == "" {
				t.Fatalf("shell authority gained by %s", name)
			}
			reason, recognized = claudeagents.GuardTool(name, "Read", []byte(`{"file_path":"note.md"}`), workspace, workspace)
			if !recognized || (reason == "") != (id == "case-agent") {
				t.Fatalf("wrong file tools for %s: %s", name, reason)
			}
		}
	}
	for _, name := range []string{"maestro", "maestro-product:maestro", "maestro-product:unknown", "other:yoda", "spoof-maestro-product:yoda", "maestro-product:other:yoda", "maestro-product:yoda:extra", " maestro-product:yoda", "maestro-product:yoda\n"} {
		id, ok := claudeagents.CanonicalAgentID(name)
		if ok || id != "" || claudeagents.Managed(name) {
			t.Fatalf("spoofed specialist admitted %q", name)
		}
	}
}

func TestOwnershipRecognizesLegacyAndCurrentHeaderOnly(t *testing.T) {
	const marker = "<!-- BCGOS:MANAGED-CLAUDE-AGENT -->\n"
	workspace := t.TempDir()
	if _, err := claudeagents.Install(workspace); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(workspace, ".claude", "agents", "yoda.md")
	definition, err := baseagents.Definition("yoda")
	if err != nil {
		t.Fatal(err)
	}
	legacy := marker + "---\nname: yoda\ndescription: Calm owner-self proxy and senior refiner for high-leverage outputs selected by Maestro.\ntools: []\npermissionMode: plan\n---\n\n" + string(definition)
	if err := os.WriteFile(path, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	if err := claudeagents.ValidateInstall(workspace); err != nil {
		t.Fatalf("legacy owned file rejected: %v", err)
	}
	if _, err := claudeagents.Inspect(workspace); err == nil {
		t.Fatal("old layout reported current")
	}
	if _, err := claudeagents.Install(workspace); err != nil {
		t.Fatalf("legacy upgrade: %v", err)
	}
	upgraded, err := os.ReadFile(path)
	if err != nil || !strings.HasPrefix(string(upgraded), "---\nname: yoda\n") {
		t.Fatal("legacy upgrade did not produce native frontmatter")
	}
	if _, err := claudeagents.Install(workspace); err != nil {
		t.Fatal(err)
	}
	if _, err := claudeagents.Inspect(workspace); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := claudeagents.Uninstall(workspace); err != nil {
		t.Fatalf("legacy uninstall: %v", err)
	}
	for _, body := range []string{
		"user prose\n" + marker,
		"---\nname: yoda\n---\n\nuser prose\n" + marker,
		strings.Replace(string(upgraded), "name: yoda", "name: personal-yoda", 1),
		strings.Replace(string(upgraded), marker, "", 1),
	} {
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := claudeagents.Install(workspace); err == nil {
			t.Fatal("body comment or unowned header claimed management")
		}
		if _, err := claudeagents.Uninstall(workspace); err == nil {
			t.Fatal("unowned header removed")
		}
		preserved, err := os.ReadFile(path)
		if err != nil || string(preserved) != body {
			t.Fatal("user-owned collision changed")
		}
	}
}
