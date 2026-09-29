package zipruntime

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestWholeContextCap(t *testing.T) {
	for _, cap := range []string{"1", "100", "30000"} {
		var out bytes.Buffer
		root := t.TempDir()
		if err := capContext(root, cap, strings.NewReader(strings.Repeat("ação\n", 20000)), &out); err != nil {
			t.Fatal(err)
		}
		var receipt map[string]any
		b, err := os.ReadFile(filepath.Join(root, "brain/.maestro/context-packet.json"))
		if err != nil {
			t.Fatal(err)
		}
		json.Unmarshal(b, &receipt)
		if !utf8.Valid(out.Bytes()) || receipt["session_stdout_bytes"] != float64(out.Len()) || float64(out.Len()) > receipt["cap_bytes"].(float64) {
			t.Fatal(out.Len(), receipt)
		}
	}
}
