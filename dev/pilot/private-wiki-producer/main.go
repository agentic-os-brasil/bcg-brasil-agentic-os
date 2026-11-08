// This development-only producer exercises real compilation from a compatible build.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/memory"
)

type sourceConfig struct {
	Policy    memory.Policy            `json:"policy"`
	Budgets   map[string]int           `json:"budgets"`
	Freshness map[string]time.Duration `json:"freshness"`
}

func readConfig(input io.Reader) (sourceConfig, error) {
	var config sourceConfig
	body, err := io.ReadAll(io.LimitReader(input, (1<<20)+1))
	if err != nil {
		return config, err
	}
	if len(body) > 1<<20 {
		return config, fmt.Errorf("producer configuration exceeds 1 MiB")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return config, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return config, fmt.Errorf("producer configuration must contain exactly one JSON object")
	}
	if err := config.Policy.Validate(); err != nil {
		return config, err
	}
	if len(config.Budgets) != 4 {
		return config, fmt.Errorf("exactly four source layer budgets are required")
	}
	for _, layer := range []string{"L1", "L2", "L3", "lifetime"} {
		if config.Budgets[layer] <= 0 {
			return config, fmt.Errorf("positive source budget required for %s", layer)
		}
	}
	for layer, freshness := range config.Freshness {
		if _, known := config.Budgets[layer]; !known || freshness < 0 {
			return config, fmt.Errorf("invalid source freshness layer or duration")
		}
	}
	return config, nil
}

func run(args []string, output io.Writer) error {
	if len(args) != 2 || !filepath.IsAbs(args[0]) || !filepath.IsAbs(args[1]) {
		return fmt.Errorf("usage: private-wiki-producer ABSOLUTE_ROOT ABSOLUTE_CONFIG")
	}
	file, err := os.Open(args[1])
	if err != nil {
		return err
	}
	defer file.Close()
	config, err := readConfig(file)
	if err != nil {
		return err
	}
	engine := memory.Engine{Root: args[0], Policy: config.Policy, Budgets: config.Budgets, Freshness: config.Freshness}
	result, err := engine.CompilePrivateWiki(context.Background(), "case-a", "")
	if err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(result)
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
