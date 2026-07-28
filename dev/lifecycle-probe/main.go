// Command lifecycle-probe captures only local environment blockers for
// lifecycle conformance. It never starts a model session, modifies runtime
// configuration, writes receipts or promotes capabilities.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/dev/lifecycleprobe"
)

func main() {
	flags := flag.NewFlagSet("lifecycle-probe", flag.ExitOnError)
	runtime := flags.String("runtime", "", "runtime to inspect: claude or codex")
	flags.Parse(os.Args[1:])
	if flags.NArg() != 0 || (*runtime != "claude" && *runtime != "codex") {
		fmt.Fprintln(os.Stderr, "usage: go run ./dev/lifecycle-probe --runtime claude|codex")
		os.Exit(2)
	}
	result, err := lifecycleprobe.SystemProbe(*runtime)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	output, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	fmt.Println(string(output))
}
