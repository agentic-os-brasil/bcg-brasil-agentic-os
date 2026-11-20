package docling

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestBoundedOutputEnforcesQuotaForOptimizedWriters(t *testing.T) {
	writers := []struct {
		name  string
		write func(io.Writer, string) (int64, error)
	}{
		{"reader_copy", func(dst io.Writer, body string) (int64, error) {
			return io.Copy(dst, io.LimitReader(strings.NewReader(body), int64(len(body))))
		}},
		{"string_write", func(dst io.Writer, body string) (int64, error) {
			n, err := io.WriteString(dst, body)
			return int64(n), err
		}},
		{"writer_to_copy", func(dst io.Writer, body string) (int64, error) {
			return io.Copy(dst, strings.NewReader(body))
		}},
	}
	for _, writer := range writers {
		t.Run(writer.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			output := &boundedOutput{maximum: 4, cancel: cancel}
			if n, err := writer.write(output, "okay"); n != 4 || err != nil || ctx.Err() != nil || string(output.Bytes()) != "okay" {
				t.Fatalf("output within quota rejected: n=%d err=%v cancelled=%t retained_bytes=%d", n, err, ctx.Err() != nil, output.Len())
			}
			n, err := writer.write(output, "!")
			if n != 0 || err == nil || err.Error() != "process_output_limit" || ctx.Err() != context.Canceled || string(output.Bytes()) != "okay" || output.Len() > 4 {
				t.Fatalf("output quota not enforced: n=%d err=%v cancelled=%t retained_bytes=%d", n, err, ctx.Err() != nil, output.Len())
			}
		})
	}
}
