// Command private-wiki-producer is an acceptance fixture, never a product tool.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/memory"
)

var sourceRevision, sourceArchiveSHA256, fixtureSHA256 string

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) != 3 {
		return errors.New("expected memory root and exact config")
	}
	body, err := os.ReadFile(os.Args[2])
	if err != nil {
		return err
	}
	var cfg struct {
		Policy    memory.Policy            `json:"policy"`
		Budgets   map[string]int           `json:"budgets"`
		Freshness map[string]time.Duration `json:"freshness"`
	}
	if err := json.Unmarshal(body, &cfg); err != nil {
		return err
	}
	engine := memory.Engine{Root: os.Args[1], Policy: cfg.Policy, Budgets: cfg.Budgets, Freshness: cfg.Freshness}
	compiled, err := engine.CompilePrivateWiki(context.Background(), "case-a", "")
	if err != nil {
		return err
	}
	image, err := os.Executable()
	if err != nil {
		return err
	}
	program, err := os.ReadFile(image)
	if err != nil {
		return err
	}
	imageHash, configHash := sha256.Sum256(program), sha256.Sum256(body)
	return json.NewEncoder(os.Stdout).Encode(struct {
		memory.PrivateWikiResult
		SourceRevision      string `json:"source_revision"`
		SourceArchiveSHA256 string `json:"source_archive_sha256"`
		FixtureSHA256       string `json:"fixture_sha256"`
		ExecutableSHA256    string `json:"executable_sha256"`
		ConfigSHA256        string `json:"config_sha256"`
	}{compiled, sourceRevision, sourceArchiveSHA256, fixtureSHA256, hex.EncodeToString(imageHash[:]), hex.EncodeToString(configHash[:])})
}
