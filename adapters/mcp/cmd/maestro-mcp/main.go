// maestro-mcp explicitly initializes a fresh binding or serves an admitted one.
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
	"os"
	"path/filepath"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "maestro: operation unavailable; inspect the selected binding before retrying")
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
	flags.StringVar(&destination, "destination", "", "new packet destination")
	flags.StringVar(&server, "server", "", "compiled native server binary")
	flags.StringVar(&version, "version", "", "packet semantic version")
	flags.StringVar(&o.ProductRoot, "product-root", "", "existing installed product directory")
	flags.StringVar(&o.UserHome, "user-home", "", "existing owner directory")
	flags.StringVar(&o.WorkspacePath, "workspace", "", "explicit selected workspace directory")
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
		home, err := datahome.InitializeFresh(datahome.Options{ProductRoot: o.ProductRoot, UserHome: o.UserHome})
		if err != nil {
			return err
		}
		w, err := workspace.Initialize(workspace.Options{WorkspacePath: o.WorkspacePath, DataRoot: filepath.Join(home.Home, "state")})
		if err != nil {
			return err
		}
		if _, err := product.Open(o); err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]string{"state": "ready", "workspace_id": w.WorkspaceID})
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
