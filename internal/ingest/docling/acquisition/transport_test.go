package acquisition

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest/docling"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestInputFetchAuthenticatesBoundedResponseAndNeverLeaksUpstreamDetails(t *testing.T) {
	body := []byte("public fixture")
	input := docling.Input{URL: "https://files.pythonhosted.org/packages/fixture.whl", Filename: "fixture.whl", SHA256: digest(body), Size: int64(len(body))}
	for _, name := range []string{"valid", "short", "oversize", "hash", "encoding", "status", "length", "transport"} {
		t.Run(name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "private")
			if err := privateDirectory(root, true); err != nil {
				t.Fatal(err)
			}
			calls := 0
			client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
				calls++
				if request.Header.Get("Authorization") != "" || request.Header.Get("Cookie") != "" || request.Header.Get("Referer") != "" || len(request.Header) != 1 {
					t.Fatal("unexpected upstream headers")
				}
				if name == "transport" {
					return nil, errors.New("secret signed URL token")
				}
				payload := append([]byte(nil), body...)
				header := http.Header{}
				status := 200
				length := int64(-1)
				switch name {
				case "short":
					payload = payload[:2]
				case "oversize":
					payload = append(payload, 'x')
				case "hash":
					payload[0] ^= 1
				case "encoding":
					header.Set("Content-Encoding", "gzip")
				case "status":
					status = 503
				case "length":
					length = input.Size + 1
				}
				return &http.Response{StatusCode: status, Header: header, ContentLength: length, Body: io.NopCloser(bytes.NewReader(payload)), Request: request}, nil
			})}
			output := filepath.Join(root, "input.part")
			err := fetchInput(context.Background(), input, output, client)
			if name == "valid" {
				if err != nil {
					t.Fatal(err)
				}
				got, _ := os.ReadFile(output)
				if !bytes.Equal(got, body) {
					t.Fatal("wrong bytes")
				}
			} else if err == nil || strings.Contains(err.Error(), "secret") {
				t.Fatalf("invalid response or sensitive detail accepted: %v", err)
			}
			if calls != 1 {
				t.Fatal("hidden retry")
			}
		})
	}
}
func TestSnapshotRequiresExactArchiveAndWholeDirectoryVerification(t *testing.T) {
	for _, valid := range []bool{true, false} {
		t.Run(map[bool]string{true: "whole_verified", false: "archive_mismatch"}[valid], func(t *testing.T) {
			f := fixture(t)
			p, err := Review(context.Background(), f.options)
			if err != nil {
				t.Fatal(err)
			}
			if err = privateDirectory(f.options.CacheRoot, true); err != nil {
				t.Fatal(err)
			}
			attempt := filepath.Join(f.options.CacheRoot, "attempt-0")
			if err = privateDirectory(attempt, true); err != nil {
				t.Fatal(err)
			}
			body := []byte("expected assembled archive")
			if !valid {
				body = []byte("wrong")
			}
			archive := filepath.Join(attempt, "archive")
			writeFixture(t, archive, body)
			verified, err := completeSnapshot(context.Background(), f.options, p, attempt, archive)
			if valid {
				if err != nil {
					t.Fatal(err)
				}
				if verified.ManifestSHA256 != f.recipe.ManifestSHA256 {
					t.Fatal("whole manifest not preserved")
				}
			} else if err == nil {
				t.Fatal("wrong deterministic artifact accepted")
			}
			if _, err = os.Lstat(filepath.Join(f.options.ReleaseDirectory, f.recipe.RuntimeArtifact.Name)); !os.IsNotExist(err) {
				t.Fatal("original carrier changed")
			}
		})
	}
}
