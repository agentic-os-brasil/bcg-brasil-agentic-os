//go:build ignore

// Run from adapters/mcp with the existing SDK module; this is not shipped to participants.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	binary := flag.String("binary", "", "ordinary actual server")
	root := flag.String("root", "", "fresh prepared synthetic pilot root")
	report := flag.String("report", "", "new proof JSON")
	flag.Parse()
	if *binary == "" || *root == "" || *report == "" {
		panic("binary, root and report required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	const summary = "MAESTRO_PILOT_2026_11_05"
	const accepted = "MAESTRO_PILOT_ACCEPTED_2026_11_05: preserve the explicit next step"
	const declined = "MAESTRO_PILOT_DECLINED_2026_11_05: this must not persist"
	const authoredMarker = "MAESTRO_PILOT_AUTHORED_2026_11_06"
	var reviews []string
	var admittedWorkspace string
	connect := func(server string, approve bool) *mcp.ClientSession {
		command := exec.CommandContext(ctx, server, "serve", "--product-root", filepath.Join(*root, "product"), "--user-home", filepath.Join(*root, "user"), "--workspace", filepath.Join(*root, "workspace"))
		options := &mcp.ClientOptions{ElicitationHandler: func(_ context.Context, request *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			reviews = append(reviews, request.Params.Message)
			knownWikiReview := admittedWorkspace != "" && strings.Contains(request.Params.Message, admittedWorkspace) && strings.Contains(request.Params.Message, "wiki-incremental-sync")
			if approve && (strings.Contains(request.Params.Message, accepted) || strings.Contains(request.Params.Message, authoredMarker) || knownWikiReview) {
				return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
			}
			return &mcp.ElicitResult{Action: "decline"}, nil
		}}
		session, err := mcp.NewClient(&mcp.Implementation{Name: "maestro-pilot-sdk-witness", Version: "1"}, options).Connect(ctx, &mcp.CommandTransport{Command: command, TerminateDuration: time.Second}, nil)
		must(err)
		if session.InitializeResult().ServerInfo.Version != "unavailable" {
			panic("ordinary server was relabelled as qualified release")
		}
		return session
	}
	call := func(session *mcp.ClientSession, tool string, args map[string]any, wantError bool) map[string]any {
		out, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: args})
		must(err)
		if out.IsError != wantError || len(out.Content) == 0 {
			panic(fmt.Sprintf("%s %v: %+v", tool, args["operation"], out))
		}
		body, ok := out.Content[0].(*mcp.TextContent)
		if !ok {
			panic("text result required")
		}
		var value map[string]any
		must(json.Unmarshal([]byte(body.Text), &value))
		return value
	}
	first := connect(*binary, true)
	tools, err := first.ListTools(ctx, nil)
	must(err)
	seen := map[string]bool{}
	for _, tool := range tools.Tools {
		seen[tool.Name] = true
	}
	for _, name := range []string{"maestro_context", "maestro_query", "maestro_apply"} {
		if !seen[name] {
			panic("missing tool: " + name)
		}
	}
	metadata := call(first, "maestro_context", map[string]any{}, false)
	workspace, ok := metadata["workspace_id"].(string)
	if !ok || workspace == "" {
		panic("missing workspace identity")
	}
	admittedWorkspace = workspace
	created := call(first, "maestro_apply", map[string]any{"operation": "execution.create", "objective": "Ensaio de continuidade do Maestro", "next_step": "Retomar no Desktop", "artifacts": []string{"EXEMPLO.md"}}, false)
	id := created["item_id"].(string)
	mutate := func(session *mcp.ClientSession, operation string, revision any, attempt string, extra map[string]any, wantError bool) map[string]any {
		args := map[string]any{"operation": "execution." + operation, "item_id": id, "expected_revision": revision}
		if attempt != "" {
			args["attempt_id"] = attempt
		}
		for key, value := range extra {
			args[key] = value
		}
		return call(session, "maestro_apply", args, wantError)
	}
	started := mutate(first, "start", float64(1), "", nil, false)
	attempt := started["attempt_id"].(string)
	checkpoint := mutate(first, "checkpoint", started["state_revision"], attempt, map[string]any{"summary": summary, "next_step": "Retomar no Desktop"}, false)
	paused := mutate(first, "pause", checkpoint["state_revision"], attempt, nil, false)
	if paused["state"] != "paused" {
		panic("work did not pause")
	}
	capture := call(first, "maestro_apply", map[string]any{"operation": "memory.capture", "text": accepted}, false)
	if capture["eligible"] != true || len(reviews) != 1 || !strings.Contains(reviews[0], accepted) {
		panic("capture did not use exact reviewed source")
	}
	methods := call(first, "maestro_query", map[string]any{"operation": "methods.list", "limit": 1}, false)
	if methods["total"] != float64(92) {
		panic("actual packet cannot discover the canonical method catalog")
	}
	const recipeURI = "maestro://methods/maestro-operator/resources/authored-brain.md"
	recipe, err := os.ReadFile(filepath.Join(*root, "exports/code-marketplace/plugins/maestro-product/skills/maestro-operator/resources/authored-brain.md"))
	must(err)
	var retrieved strings.Builder
	for offset := 0; ; {
		page := call(first, "maestro_query", map[string]any{"operation": "methods.read", "resource_uri": recipeURI, "offset": offset, "max_bytes": 4096}, false)
		if page["sha256"] != fmt.Sprintf("%x", sha256.Sum256(recipe)) || page["total_bytes"] != float64(len(recipe)) {
			panic("immutable authored recipe differs from the distributed packet")
		}
		retrieved.WriteString(page["text"].(string))
		if page["has_more"] == false {
			break
		}
		next := int(page["next_offset"].(float64))
		if next <= offset {
			panic("method resource paging did not advance")
		}
		offset = next
	}
	if retrieved.String() != string(recipe) {
		panic("canonical method bytes were not completely recovered")
	}
	plan := call(first, "maestro_query", map[string]any{"operation": "methods.plan", "method_id": "maestro-operator"}, false)
	if plan["execution_state"] != "not_executed" {
		panic("method selection claimed execution")
	}
	const authoredPage = "projects/pilot-shared.md"
	authoredBody := fmt.Sprintf("---\nid: projects/pilot-shared\ntitle: Reviewed pilot page\nsummary: A synthetic page shared by the pilot clients.\ntype: project-brief\nscope: workspace/%s\nstatus: draft\nsensitivity: internal\nupdated: %s\n---\n# Reviewed pilot page\n\n%s\n", workspace, time.Now().UTC().Format("2006-01-02"), authoredMarker)
	authored := call(first, "maestro_apply", map[string]any{"operation": "brain.create", "page": authoredPage, "text": authoredBody}, false)
	if authored["state"] != "written" {
		panic("actual kit did not persist its reviewed authored page")
	}
	call(first, "maestro_apply", map[string]any{"operation": "memory.consolidate", "cycle": "daily", "date": time.Now().UTC().Format("2006-01-02")}, false)
	qualification := call(first, "maestro_apply", map[string]any{"operation": "maintenance.qualify", "job_id": "wiki-incremental-sync", "expected_revision": 0}, false)
	if qualification["qualification_state"] != "qualified" || qualification["state"] != "inactive" {
		panic("kit did not qualify private wiki without activating recurring work")
	}
	wiki := call(first, "maestro_query", map[string]any{"operation": "wiki.status"}, false)
	wikiGeneration, ok := wiki["generation"].(string)
	if !ok || wikiGeneration == "" || wiki["state"] != "current" {
		panic("actual committed memory did not yield a current private wiki")
	}
	must(first.Close())
	packetName := "maestro-mcp"
	if runtime.GOOS == "windows" {
		packetName += ".exe"
	}
	packetBinary := filepath.Join(*root, "exports", "code-marketplace", "plugins", "maestro-product", "scripts", packetName)
	firstBytes, err := os.ReadFile(*binary)
	must(err)
	packetBytes, err := os.ReadFile(packetBinary)
	must(err)
	if sha256.Sum256(firstBytes) != sha256.Sum256(packetBytes) {
		panic("packet server differs from ordinary build")
	}
	second := connect(packetBinary, true)
	defer second.Close()
	secondMetadata := call(second, "maestro_context", map[string]any{}, false)
	if secondMetadata["workspace_id"] != workspace {
		panic("packet process changed workspace")
	}
	pageAfterRestart := call(second, "maestro_query", map[string]any{"operation": "brain.read", "page": authoredPage, "purpose": "Continue the reviewed pilot page in the copied packet server"}, false)
	if pageAfterRestart["text"] != authoredBody || pageAfterRestart["revision"] != authored["revision"] {
		panic("packet server lost reviewed authored bytes or revision")
	}
	wikiAfterRestart := call(second, "maestro_query", map[string]any{"operation": "wiki.status"}, false)
	if wikiAfterRestart["generation"] != wikiGeneration || wikiAfterRestart["state"] != "current" {
		panic("packet server lost the admitted private wiki generation")
	}
	wikiIndex := call(second, "maestro_query", map[string]any{"operation": "wiki.read", "generation": wikiGeneration, "reference": "index.md", "max_bytes": 4096}, false)
	if strings.TrimSpace(wikiIndex["content"].(string)) == "" {
		panic("packet private wiki index is empty")
	}
	list := call(second, "maestro_query", map[string]any{"operation": "execution.list", "limit": 32}, false)
	var found map[string]any
	for _, row := range list["items"].([]any) {
		value := row.(map[string]any)
		if value["item_id"] == id {
			found = value
		}
	}
	if found == nil || found["state"] != "paused" || found["state_revision"] != paused["state_revision"] {
		panic("fresh packet process lost exact paused item")
	}
	resumed := mutate(second, "resume", paused["state_revision"], "", nil, false)
	if resumed["state"] != "running" || resumed["attempt_id"] == attempt {
		panic("resume did not create a fresh attempt")
	}
	mutate(second, "checkpoint", resumed["state_revision"], attempt, map[string]any{"summary": "stale", "next_step": "must refuse"}, true)
	next := call(second, "maestro_query", map[string]any{"operation": "execution.next", "item_id": id}, false)
	if next["next_step"] != "Retomar no Desktop" {
		panic("continuation changed")
	}
	read := call(second, "maestro_query", map[string]any{"operation": "memory.read", "capture_id": capture["id"]}, false)
	if read["text"] != accepted {
		panic("reviewed capture lost after packet restart")
	}
	before := snapshot(filepath.Join(*root, "user"))
	declining := connect(packetBinary, false)
	call(declining, "maestro_apply", map[string]any{"operation": "memory.capture", "text": declined}, true)
	must(declining.Close())
	if !reflect.DeepEqual(before, snapshot(filepath.Join(*root, "user"))) {
		panic("declined review changed canonical owner state")
	}
	search := call(second, "maestro_query", map[string]any{"operation": "memory.search", "query": declined}, false)
	if search["total"] != float64(0) {
		panic("declined source became visible")
	}
	identity := exec.CommandContext(ctx, *binary, "identity")
	identityOutput, identityError := identity.CombinedOutput()
	if identityError == nil {
		panic("developer binary unexpectedly has qualified modern identity")
	}
	proof := map[string]any{"passed": true, "mode": "ordinary_unversioned_developer_rehearsal", "sdk": "official_go_sdk_v1.8.0", "transport": "actual_shipping_stdio_and_packet_server", "server_version": second.InitializeResult().ServerInfo.Version, "workspace_id": workspace, "item_id": id, "paused": paused, "resumed": resumed, "checkpoint_literal": summary, "capture_id": capture["id"], "exact_review_count": len(reviews), "declined_owner_bytes_unchanged": true, "modern_identity_refused": true, "modern_identity_output": string(identityOutput), "native_Code_Desktop": "not_run", "host_review_evidence": "synthetic_official_SDK_form_witness_not_native_human_acceptance"}
	proof["methods_total"] = methods["total"]
	proof["canonical_authored_recipe_recovered"] = true
	proof["authored_page_same_revision_after_packet_restart"] = true
	proof["private_wiki_generation_after_packet_restart"] = wikiGeneration
	proof["recurring_work_activated"] = false
	body, err := json.MarshalIndent(proof, "", "  ")
	must(err)
	must(os.WriteFile(*report, append(body, '\n'), 0600))
	fmt.Println(string(body))
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func snapshot(root string) map[string]string {
	out := map[string]string{}
	must(filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !entry.IsDir() && !info.Mode().IsRegular() {
			return fmt.Errorf("unexpected private proof entry")
		}
		name, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		value := info.Mode().String()
		if !entry.IsDir() {
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			value += fmt.Sprintf("/%x", sha256.Sum256(body))
		}
		out[name] = value
		return nil
	}))
	return out
}
