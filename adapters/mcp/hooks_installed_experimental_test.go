//go:build maestro_experimental_installed_seed

package product

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestInstalledClaudeHookRetainsConstructorAndCancellationRefusals(t *testing.T) {
	for _, channel := range []string{"native-first", "model-approved", "LOCAL-BROWSER", "local-browser\n"} {
		input := &hookInputObserver{Reader: strings.NewReader("PRIVATE_EVENT")}
		var output bytes.Buffer
		err := HandleInstalledClaudeHook(context.Background(), Options{OwnerReviewChannel: channel}, "PreToolUse", strings.Repeat("a", 64), strings.Repeat("b", 32), input, &output)
		if err == nil || err.Error() != "owner_review_channel_unavailable" || input.reads != 0 || output.Len() != 0 {
			t.Fatalf("review option reached admission/input: %q %v reads=%d output=%q", channel, err, input.reads, output.String())
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	input := &hookInputObserver{Reader: strings.NewReader("PRIVATE_EVENT")}
	var output bytes.Buffer
	err := HandleInstalledClaudeHook(ctx, Options{}, "PreToolUse", strings.Repeat("a", 64), strings.Repeat("b", 32), input, &output)
	if !errors.Is(err, context.Canceled) || input.reads != 0 || output.Len() != 0 {
		t.Fatalf("cancelled installed hook reached input: %v reads=%d output=%q", err, input.reads, output.String())
	}
}

type installedHookReaderFunc func([]byte) (int, error)

func (f installedHookReaderFunc) Read(body []byte) (int, error) { return f(body) }

type installedHookWriterFunc func([]byte) (int, error)

func (f installedHookWriterFunc) Write(body []byte) (int, error) { return f(body) }

func TestInstalledHookCancellationAndIOStayClosed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	read, wrote := false, false
	input := installedHookInput{ctx, installedHookReaderFunc(func([]byte) (int, error) { read = true; return 0, io.EOF })}
	output := installedHookOutput{ctx, installedHookWriterFunc(func(body []byte) (int, error) { wrote = true; return len(body), nil })}
	if n, err := input.Read(make([]byte, 8)); n != 0 || !errors.Is(err, context.Canceled) || read {
		t.Fatalf("cancelled read consumed event bytes: n=%d err=%v read=%v", n, err, read)
	}
	if n, err := output.Write([]byte("{}\n")); n != 0 || !errors.Is(err, context.Canceled) || wrote {
		t.Fatalf("cancelled write emitted success: n=%d err=%v wrote=%v", n, err, wrote)
	}
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	input = installedHookInput{ctx, installedHookReaderFunc(func(body []byte) (int, error) {
		cancel()
		return copy(body, "PRIVATE"), nil
	})}
	if _, err := io.ReadAll(input); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation during input became valid event bytes: %v", err)
	}
	sentinel := errors.New("synthetic reader failure")
	input = installedHookInput{context.Background(), installedHookReaderFunc(func([]byte) (int, error) { return 0, sentinel })}
	if _, err := io.ReadAll(input); !errors.Is(err, sentinel) {
		t.Fatal("input failure was lost", err)
	}
	output = installedHookOutput{context.Background(), installedHookWriterFunc(func([]byte) (int, error) { return 0, sentinel })}
	if _, err := output.Write([]byte("{}\n")); !errors.Is(err, sentinel) {
		t.Fatal("output failure was lost", err)
	}
}
