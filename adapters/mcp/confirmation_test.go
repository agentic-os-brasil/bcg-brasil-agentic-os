package product

import "testing"

func TestConfirmationProtocolLabelIsBounded(t *testing.T) {
	for _, tc := range []struct{ version, want string }{
		{"2025-11-25", "2025-11-25"},
		{"2026-07-28", "2026-07-28"},
		{"", "missing"},
		{"unknown\nprivate-protocol-marker /private/path", "other"},
	} {
		if got := confirmationProtocolLabel(tc.version); got != tc.want {
			t.Fatalf("protocol label = %q; want %q", got, tc.want)
		}
	}
}
