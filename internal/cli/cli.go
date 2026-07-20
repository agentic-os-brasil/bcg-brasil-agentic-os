package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	basememory "github.com/DScardini91/bcg-brasil-agentic-os/bundles/base/memory"
	"github.com/DScardini91/bcg-brasil-agentic-os/internal/memory"
)

const (
	ExitOK          = 0
	ExitFailure     = 1
	ExitUsage       = 2
	ExitUnavailable = 3
)

var Version = "0.0.0-dev"

func Run(args []string, out, errOut io.Writer) int {
	return RunWithInput(args, strings.NewReader(""), out, errOut)
}

func RunWithInput(args []string, in io.Reader, out, errOut io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "usage: bcgos <version|memory>")
		return ExitUsage
	}
	switch args[0] {
	case "help", "--help", "-h":
		fmt.Fprintln(out, "usage: bcgos <version|memory>")
		return ExitOK
	case "version":
		fmt.Fprintf(out, "bcgos %s\n", Version)
		return ExitOK
	case "memory":
		return runMemory(args[1:], in, out, errOut)
	default:
		fmt.Fprintf(errOut, "unknown command %q\n", args[0])
		return ExitUsage
	}
}

func runMemory(args []string, in io.Reader, out, errOut io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "usage: bcgos memory <capture|status|context|dream>")
		return ExitUsage
	}
	switch args[0] {
	case "help", "--help", "-h":
		fmt.Fprintln(out, "usage: bcgos memory <capture|status|context|dream>")
		return ExitOK
	case "capture":
		return runCapture(args[1:], in, out, errOut)
	case "status":
		return runStatus(args[1:], out, errOut)
	case "context":
		return runContext(args[1:], out, errOut)
	case "dream":
		return runDream(args[1:], out, errOut)
	default:
		fmt.Fprintf(errOut, "unknown memory command %q\n", args[0])
		return ExitUsage
	}
}

func runCapture(args []string, in io.Reader, out, errOut io.Writer) int {
	flags := newFlagSet("memory capture", errOut)
	dataDir := flags.String("data-dir", "", "local BCGOS data directory")
	workspace := flags.String("workspace", "", "workspace identity")
	kind := flags.String("kind", "", "sanitized signal kind")
	stdin := flags.Bool("stdin", false, "read sanitized signal text from standard input")
	sanitized := flags.Bool("sanitized", false, "attest that adapter sanitization has completed")
	if err := flags.Parse(args); err != nil {
		return ExitUsage
	}
	if rejectPositionals(flags, errOut) {
		return ExitUsage
	}
	if missing := required(map[string]string{"--data-dir": *dataDir, "--workspace": *workspace, "--kind": *kind}); missing != "" {
		fmt.Fprintf(errOut, "%s is required\n", missing)
		return ExitUsage
	}
	if !*sanitized {
		fmt.Fprintln(errOut, "--sanitized is required; raw input must not be persisted")
		return ExitUsage
	}
	if !*stdin {
		fmt.Fprintln(errOut, "--stdin is required; professional content must not be passed in process arguments")
		return ExitUsage
	}
	const maximumCaptureBytes = 1 << 20
	content, err := io.ReadAll(io.LimitReader(in, maximumCaptureBytes+1))
	if err != nil {
		return reportError(errOut, err)
	}
	if len(content) > maximumCaptureBytes {
		return reportError(errOut, errors.New("capture exceeds 1 MiB limit"))
	}
	text := strings.TrimSpace(string(content))
	if text == "" {
		fmt.Fprintln(errOut, "standard input is empty")
		return ExitUsage
	}
	policy, err := basememory.Policy()
	if err != nil {
		return reportError(errOut, err)
	}
	engine := memory.Engine{Root: filepath.Join(*dataDir, "memory"), Policy: policy}
	path, err := engine.Capture(memory.Capture{WorkspaceID: *workspace, RecordedAt: time.Now().UTC(), Kind: *kind, Text: text, Sanitized: true})
	if err != nil {
		return reportError(errOut, err)
	}
	return writeJSON(out, map[string]any{"workspace_id": *workspace, "state": "captured", "path": path}, errOut)
}

func runStatus(args []string, out, errOut io.Writer) int {
	flags := newFlagSet("memory status", errOut)
	dataDir := flags.String("data-dir", "", "local BCGOS data directory")
	workspace := flags.String("workspace", "", "workspace identity")
	if err := flags.Parse(args); err != nil {
		return ExitUsage
	}
	if rejectPositionals(flags, errOut) {
		return ExitUsage
	}
	if missing := required(map[string]string{"--data-dir": *dataDir, "--workspace": *workspace}); missing != "" {
		fmt.Fprintf(errOut, "%s is required\n", missing)
		return ExitUsage
	}
	engine := memory.Engine{Root: filepath.Join(*dataDir, "memory")}
	report, err := engine.Status(*workspace)
	if err != nil {
		return reportError(errOut, err)
	}
	return writeJSON(out, struct {
		memory.StatusReport
		Dreaming string `json:"dreaming"`
	}{StatusReport: report, Dreaming: "unavailable"}, errOut)
}

func runContext(args []string, out, errOut io.Writer) int {
	flags := newFlagSet("memory context", errOut)
	dataDir := flags.String("data-dir", "", "local BCGOS data directory")
	workspace := flags.String("workspace", "", "workspace identity")
	l1 := flags.Int("budget-l1", 0, "maximum L1 characters")
	l2 := flags.Int("budget-l2", 0, "maximum L2 characters")
	l3 := flags.Int("budget-l3", 0, "maximum L3 characters")
	lifetime := flags.Int("budget-lifetime", 0, "maximum lifetime characters")
	if err := flags.Parse(args); err != nil {
		return ExitUsage
	}
	if rejectPositionals(flags, errOut) {
		return ExitUsage
	}
	if missing := required(map[string]string{"--data-dir": *dataDir, "--workspace": *workspace}); missing != "" {
		fmt.Fprintf(errOut, "%s is required\n", missing)
		return ExitUsage
	}
	budgets := map[string]int{"L1": *l1, "L2": *l2, "L3": *l3, "lifetime": *lifetime}
	for layer, budget := range budgets {
		if budget <= 0 {
			fmt.Fprintf(errOut, "positive budget required for %s\n", layer)
			return ExitUsage
		}
	}
	policy, err := basememory.Policy()
	if err != nil {
		return reportError(errOut, err)
	}
	engine := memory.Engine{Root: filepath.Join(*dataDir, "memory"), Policy: policy, Budgets: budgets}
	bundle, err := engine.AssembleContext(*workspace)
	if err != nil {
		return reportError(errOut, err)
	}
	return writeJSON(out, bundle, errOut)
}

func runDream(args []string, out, errOut io.Writer) int {
	if len(args) == 0 || (args[0] != "daily" && args[0] != "weekly") {
		fmt.Fprintln(errOut, "usage: bcgos memory dream <daily|weekly> --data-dir PATH --workspace ID")
		return ExitUsage
	}
	cycle := args[0]
	flags := newFlagSet("memory dream "+cycle, errOut)
	dataDir := flags.String("data-dir", "", "local BCGOS data directory")
	workspace := flags.String("workspace", "", "workspace identity")
	if err := flags.Parse(args[1:]); err != nil {
		return ExitUsage
	}
	if rejectPositionals(flags, errOut) {
		return ExitUsage
	}
	if missing := required(map[string]string{"--data-dir": *dataDir, "--workspace": *workspace}); missing != "" {
		fmt.Fprintf(errOut, "%s is required\n", missing)
		return ExitUsage
	}
	code := writeJSON(out, map[string]any{
		"capability":   "memory_dreaming",
		"cycle":        cycle,
		"state":        "unavailable",
		"workspace_id": *workspace,
		"reason":       "no synthesis and eligibility adapter is installed",
	}, errOut)
	if code != ExitOK {
		return code
	}
	return ExitUnavailable
}

func newFlagSet(name string, errOut io.Writer) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(errOut)
	return flags
}

func required(values map[string]string) string {
	for _, key := range []string{"--data-dir", "--workspace", "--kind"} {
		if value, exists := values[key]; exists && strings.TrimSpace(value) == "" {
			return key
		}
	}
	return ""
}

func rejectPositionals(flags *flag.FlagSet, errOut io.Writer) bool {
	if flags.NArg() == 0 {
		return false
	}
	fmt.Fprintf(errOut, "unexpected positional argument %q; professional content must enter through standard input\n", flags.Arg(0))
	return true
}

func reportError(errOut io.Writer, err error) int {
	if err == nil {
		return ExitOK
	}
	_ = json.NewEncoder(errOut).Encode(map[string]string{"state": "error", "error": err.Error()})
	return ExitFailure
}

func writeJSON(out io.Writer, value any, errOut io.Writer) int {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		if !errors.Is(err, io.ErrClosedPipe) {
			fmt.Fprintln(errOut, err)
		}
		return ExitFailure
	}
	return ExitOK
}
