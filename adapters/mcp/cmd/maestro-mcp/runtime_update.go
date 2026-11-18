package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
)

func runInstalledRuntime(ctx context.Context, command string, o product.Options, release, digest string, confirmed bool, output io.Writer) error {
	switch command {
	case "runtime-review":
		if digest != "" || confirmed {
			return errors.New("runtime review is read-only")
		}
		review, err := product.ReviewInstalledRuntime(ctx, o, release)
		if err != nil {
			return err
		}
		return json.NewEncoder(output).Encode(review)
	case "runtime-apply":
		result, err := product.ApplyInstalledRuntime(ctx, o, release, digest, confirmed)
		if e := json.NewEncoder(output).Encode(result); e != nil {
			return e
		}
		return err
	default:
		return errors.New("unsupported installed runtime command")
	}
}
