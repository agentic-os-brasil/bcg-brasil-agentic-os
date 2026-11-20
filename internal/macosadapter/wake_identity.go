package macosadapter

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"strings"
)

// VerifyLoadedArguments narrows OSWK ownership beyond a shared program/path.
// Unknown print formats refuse rather than authorizing an occupied service.
func VerifyLoadedArguments(output string, spec Spec) error {
	if len(output) > maxRunnerOutput {
		return errors.New("native wake identity output exceeds bound")
	}
	var observed []string
	inside, seen, closed := false, false, false
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "arguments = {" {
			if seen {
				return errors.New("ambiguous native wake arguments")
			}
			inside, seen = true, true
			continue
		}
		if !inside {
			continue
		}
		if line == "}" {
			inside, closed = false, true
			continue
		}
		if line == "" || len(line) > 4096 || len(observed) >= 64 || strings.ContainsAny(line, "\x00\r") {
			return errors.New("invalid native wake arguments")
		}
		if strings.HasPrefix(line, "\"") {
			value, err := strconv.Unquote(line)
			if err != nil || strings.ContainsAny(value, "\x00\r\n") {
				return errors.New("invalid quoted native wake argument")
			}
			line = value
		}
		observed = append(observed, line)
	}
	if !seen || !closed || inside || !reflect.DeepEqual(observed, append([]string{spec.Program}, spec.Arguments...)) {
		return errors.New("loaded native wake arguments differ from exact owned definition")
	}
	return nil
}

func (lifecycle Lifecycle) VerifyArguments(ctx context.Context, spec Spec) error {
	if !lifecycle.Native {
		return nil
	}
	if err := validateNativeLabel(spec.Label); err != nil {
		return err
	}
	result, err := lifecycle.runResult(ctx, []string{"print", "gui/" + lifecycle.UID + "/" + spec.Label})
	if err != nil {
		return err
	}
	return VerifyLoadedArguments(result.Output, spec)
}
