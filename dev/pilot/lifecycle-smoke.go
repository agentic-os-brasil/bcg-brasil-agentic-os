//go:build ignore

// This developer-only witness seeds synthetic reviewed memory through the SDK.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	binary := flag.String("binary", "", "actual prepared server")
	root := flag.String("root", "", "new synthetic prepared participant root")
	marker := flag.String("marker", "", "synthetic context marker")
	report := flag.String("report", "", "new proof JSON")
	flag.Parse()
	if *binary == "" || *root == "" || *report == "" || !regexp.MustCompile(`^[A-Z0-9_]{16,128}$`).MatchString(*marker) {
		panic("explicit binary/root/report and bounded synthetic marker required")
	}
	if _, err := os.Lstat(*report); !os.IsNotExist(err) {
		panic("preserve existing proof destination")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	binding := []string{"--product-root", filepath.Join(*root, "product"), "--user-home", filepath.Join(*root, "user"), "--workspace", filepath.Join(*root, "workspace")}
	reviews := 0
	client := mcp.NewClient(&mcp.Implementation{Name: "maestro-lifecycle-sdk-witness", Version: "1"}, &mcp.ClientOptions{
		ElicitationHandler: func(_ context.Context, r *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			if strings.Contains(r.Params.Message, *marker) {
				reviews++
				return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
			}
			return &mcp.ElicitResult{Action: "decline"}, nil
		},
	})
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: exec.CommandContext(ctx, *binary, append([]string{"serve"}, binding...)...), TerminateDuration: time.Second}, nil)
	must(err)
	defer session.Close()
	if session.InitializeResult().ServerInfo.Version != "unavailable" {
		panic("this witness requires ordinary unversioned developer mode")
	}
	call := func(tool string, args map[string]any) map[string]any {
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: args})
		must(err)
		if result.IsError || len(result.Content) != 1 {
			panic("synthetic MCP operation failed")
		}
		text, ok := result.Content[0].(*mcp.TextContent)
		if !ok {
			panic("expected actual text result")
		}
		var value map[string]any
		must(json.Unmarshal([]byte(text.Text), &value))
		return value
	}
	metadata := call("maestro_context", map[string]any{})
	listed := call("maestro_query", map[string]any{"operation": "memory.list"})
	if listed["total"] != float64(0) {
		panic("the synthetic witness requires empty reviewed memory")
	}
	capture := call("maestro_apply", map[string]any{"operation": "memory.capture", "text": *marker})
	if capture["eligible"] != true || reviews != 1 {
		panic("capture did not receive its exact synthetic SDK review")
	}
	hook := func() []byte {
		input, err := json.Marshal(map[string]any{"session_id": "lifecycle-compiled-witness", "cwd": filepath.Join(*root, "workspace"), "hook_event_name": "SessionStart"})
		must(err)
		command := exec.CommandContext(ctx, *binary, append([]string{"lifecycle-hook", "--event", "SessionStart"}, binding...)...)
		command.Stdin = bytes.NewReader(input)
		var stderr bytes.Buffer
		command.Stderr = &stderr
		body, err := command.Output()
		if err != nil {
			panic("compiled lifecycle command failed: " + stderr.String())
		}
		return body
	}
	before := snapshot(*root)
	if strings.Contains(string(hook()), *marker) || !reflect.DeepEqual(before, snapshot(*root)) {
		panic("unconsolidated capture was injected or startup mutated the fixture")
	}
	call("maestro_apply", map[string]any{"operation": "memory.consolidate", "cycle": "daily", "date": time.Now().UTC().Format("2006-01-02")})
	contextValue := call("maestro_query", map[string]any{"operation": "memory.context", "max_bytes": 4096})
	if !strings.Contains(fmt.Sprint(contextValue["text"]), *marker) {
		panic("explicit generated context lacks the reviewed marker")
	}
	must(session.Close())
	before = snapshot(*root)
	body := hook()
	if !strings.Contains(string(body), *marker) || !reflect.DeepEqual(before, snapshot(*root)) {
		panic("restarted compiled startup lost generated memory or changed files")
	}
	proof := map[string]any{
		"passed": true, "workspace_context": metadata, "capture_id": capture["id"], "marker": *marker,
		"synthetic_SDK_reviews": reviews, "human_confirmation": "not_run",
		"unconsolidated_capture_omitted": true, "explicit_daily_consolidation": true,
		"query_and_restarted_compiled_hook_match": true, "whole_tree_bytes_modes_unchanged_by_startup": true,
		"native_client": "not_run_by_this_witness", "profile": "mcp_lifecycle", "enforcement": "unavailable",
	}
	encoded, err := json.MarshalIndent(proof, "", "  ")
	must(err)
	must(os.WriteFile(*report, append(encoded, '\n'), 0600))
	fmt.Println("PASS: reviewed generated memory delivered through restarted compiled lifecycle command; native client remains separate")
}

func snapshot(root string) map[string]string {
	result := map[string]string{}
	must(filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !entry.IsDir() && !info.Mode().IsRegular() {
			return fmt.Errorf("unexpected synthetic fixture entry")
		}
		value := info.Mode().String()
		if !entry.IsDir() {
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			value += fmt.Sprintf(":%x", sha256.Sum256(body))
		}
		result[path] = value
		return nil
	}))
	return result
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
