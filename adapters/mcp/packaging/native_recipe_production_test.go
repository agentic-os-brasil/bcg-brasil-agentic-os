//go:build !maestro_experimental_installed_seed

package packaging

import (
	"bytes"
	"strings"
	"testing"
)

func TestOrdinaryCanonicalNativeRecipeBytesStayExact(t *testing.T) {
	for _, platform := range []string{"darwin", "win32"} {
		for _, binding := range []string{"", strings.Repeat("a", 64)} {
			old, err := nativeProfilePayload(platform, binding)
			if err != nil {
				t.Fatal(err)
			}
			current, err := canonicalNativeProfile(platform, binding, nil)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(old["hooks/hooks.json"], current["hooks/hooks.json"]) {
				t.Fatal("ordinary canonical wire changed")
			}
		}
	}
}
