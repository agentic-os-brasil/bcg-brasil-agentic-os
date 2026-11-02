// maestro-mcp explicitly enrolls a workspace under fresh or admitted owner state.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/packaging"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	if err := run(); err != nil {
		if len(os.Args) > 1 && strings.HasPrefix(os.Args[1], "memory-") {
			fmt.Fprintln(os.Stderr, "maestro: memory operation refused; read the current private memory-review from JSON stdin, then retry memory-apply with the same request, its --review-digest and explicit --confirm")
		} else if len(os.Args) > 1 && (os.Args[1] == "owner-apply" || os.Args[1] == "owner-confirm-onboarding") {
			fmt.Fprintln(os.Stderr, "maestro: owner confirmation refused; inspect the selected binding, read the current full owner-review or owner-onboarding-review, then retry with its exact --review-digest and --confirm")
		} else if len(os.Args) > 1 && os.Args[1] == "owner-track" {
			fmt.Fprintln(os.Stderr, "maestro: owner track selection refused; inspect the selected binding, then use --track quick|complete with explicit --confirm")
		} else {
			fmt.Fprintln(os.Stderr, "maestro: operation unavailable; inspect the selected binding before retrying")
		}
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) < 2 {
		return errors.New("command_required")
	}
	flags := flag.NewFlagSet("maestro-mcp", flag.ContinueOnError)
	var o product.Options
	var destination, server, version string
	var proposalID, reviewDigest, track string
	var confirmed bool
	flags.StringVar(&destination, "destination", "", "new packet destination")
	flags.StringVar(&server, "server", "", "compiled native server binary")
	flags.StringVar(&version, "version", "", "packet semantic version")
	flags.StringVar(&o.ProductRoot, "product-root", "", "existing installed product directory")
	flags.StringVar(&o.UserHome, "user-home", "", "existing owner directory")
	flags.StringVar(&o.WorkspacePath, "workspace", "", "explicit selected workspace directory")
	switch os.Args[1] {
	case "owner-review", "owner-apply":
		flags.StringVar(&proposalID, "proposal-id", "", "exact owner proposal ID")
	}
	switch os.Args[1] {
	case "owner-apply", "owner-confirm-onboarding", "memory-apply":
		flags.StringVar(&reviewDigest, "review-digest", "", "exact SHA-256 digest from the full local review")
	}
	switch os.Args[1] {
	case "owner-apply", "owner-confirm-onboarding", "owner-track", "memory-apply":
		flags.BoolVar(&confirmed, "confirm", false, "local operator confirms the reviewed operation")
	}
	if os.Args[1] == "owner-track" {
		flags.StringVar(&track, "track", "", "quick or complete")
	}
	if err := flags.Parse(os.Args[2:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected_arguments")
	}
	switch os.Args[1] {
	case "plugin":
		return packaging.BuildPlugin(packaging.Options{Destination: destination, Server: server, Version: version, Binding: o})
	case "mcpb":
		return packaging.BuildExtension(packaging.Options{Destination: destination, Server: server, Version: version, Binding: o})
	case "inspect":
		h, err := datahome.Resolve(datahome.Options{ProductRoot: o.ProductRoot, UserHome: o.UserHome})
		diagnostic := h.Diagnostic
		if diagnostic == "" {
			diagnostic = "unavailable"
		}
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"diagnostic": diagnostic, "writable": h.Writable, "origin": h.Origin})
		return err
	case "init":
		if err := product.Preflight(o); err != nil {
			return err
		}
		homeOptions := datahome.Options{ProductRoot: o.ProductRoot, UserHome: o.UserHome}
		home, err := datahome.Resolve(homeOptions)
		if err != nil {
			return err
		}
		if !home.Writable {
			home, err = datahome.InitializeFresh(homeOptions)
			if err != nil {
				return err
			}
		}
		w, err := workspace.Initialize(workspace.Options{WorkspacePath: o.WorkspacePath, DataRoot: filepath.Join(home.Home, "state")})
		if err != nil {
			return err
		}
		bound, err := product.Open(o)
		if err != nil {
			return err
		}
		if _, err := bound.InitializeOwner(); err != nil {
			return err
		}
		if err := bound.InitializeMemory(); err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]string{"state": "ready", "workspace_id": w.WorkspaceID})
	case "owner-review", "owner-apply", "owner-onboarding-review", "owner-confirm-onboarding", "owner-track":
		bound, err := product.Open(o)
		if err != nil {
			return err
		}
		var value any
		switch os.Args[1] {
		case "owner-review":
			value, err = bound.OwnerReview(proposalID)
		case "owner-apply":
			value, err = bound.ApplyOwnerReview(proposalID, reviewDigest, confirmed)
		case "owner-onboarding-review":
			value, err = bound.OwnerOnboardingReview()
		case "owner-confirm-onboarding":
			value, err = bound.ConfirmOwnerOnboarding(reviewDigest, confirmed)
		case "owner-track":
			value, err = bound.SelectOwnerTrack(track, confirmed)
		}
		if err != nil {
			return err
		}
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(value)
	case "memory-review", "memory-apply":
		bound, err := product.Open(o)
		if err != nil {
			return err
		}
		body, err := io.ReadAll(io.LimitReader(os.Stdin, (128<<10)+1))
		if err != nil || len(body) > 128<<10 {
			return errors.New("memory_input_size")
		}
		request, err := product.DecodeMemoryMutation(body)
		if err != nil {
			return err
		}
		var value any
		if os.Args[1] == "memory-review" {
			value, err = bound.MemoryReview(request)
		} else {
			value, err = bound.ApplyMemoryReview(request, reviewDigest, confirmed)
		}
		if err != nil {
			return err
		}
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(value)
	case "serve":
		bound, err := product.Open(o)
		if err != nil {
			return err
		}
		return bound.Server().Run(context.Background(), &mcp.StdioTransport{})
	default:
		return errors.New("unsupported_command")
	}
}
