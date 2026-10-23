// server is a dev-only synthetic architecture spike, never a production service.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"regexp"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/dev/spikes/maestromcp/spike"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func run(ctx context.Context, args []string, out, diagnostic io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: server seed|serve|orient [flags]")
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	flags.SetOutput(diagnostic)
	root := flags.String("root", "", "explicit synthetic fixture root under Developer")
	workspace := flags.String("workspace", "", "case-a or case-b, bound at process launch")
	revision := flags.String("revision", "", "synthetic plugin revision for SessionStart orientation")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	switch args[0] {
	case "seed":
		if *workspace != "" || *revision != "" {
			return errors.New("seed accepts only --root")
		}
		fixture, err := spike.Seed(*root)
		if err != nil {
			return err
		}
		return json.NewEncoder(out).Encode(fixture)
	case "serve":
		if *revision != "" {
			return errors.New("serve accepts only --root and --workspace")
		}
		bound, err := spike.Open(*root, *workspace)
		if err != nil {
			return err
		}
		return bound.Server().Run(ctx, &mcp.StdioTransport{})
	case "orient":
		if *root != "" || *workspace != "" || !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`).MatchString(*revision) {
			return errors.New("orient requires only a bounded --revision")
		}
		_, err := fmt.Fprintf(out, "MAESTRO_MCP_SPIKE_REVISION=%s\nMaestro synthetic architecture spike: use maestro_context before work. Filesystem is the source of truth. Save a concise checkpoint and next step with maestro_checkpoint; pause explicitly before a later maestro_resume. Tools are bound to one synthetic case. Never reuse another case's context or write across cases. Plugin revision is independent of user state. This is dev evidence, not production qualification.\n", *revision)
		return err
	default:
		return errors.New("unknown subcommand; use seed, serve or orient")
	}
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}
