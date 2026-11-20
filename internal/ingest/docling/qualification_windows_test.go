//go:build windows

package docling

import (
	"encoding/json"
	"strings"
	"testing"
)

// Called by the existing native-confinement parent; this adds no proof parent.
func assertWindowsQualificationFailureFactsPrivacy(t *testing.T) {
	t.Helper()
	const bridge = `C:\private\snapshot\bridge.py`
	const secret = "PRIVATE_QUALIFICATION_MARKER"
	frame := "  File \"" + bridge + "\", line "
	trace := "Traceback (most recent call last):\n"
	cases := []struct {
		name, stderr, exception string
		line                    int
	}{
		{"exact_assertion", trace + frame + "55, in <module>\n    assert denied(error)\nAssertionError: " + secret, "AssertionError", 55},
		{"chained_exception", trace + frame + "51, in <module>\nConnectionRefusedError: " + secret + "\n\nDuring handling of the above exception, another exception occurred:\n\n" + trace + frame + "55, in <module>\nAssertionError: " + secret, "AssertionError", 55},
		{"foreign_path", trace + `  File "C:\other\snapshot\bridge.py", line 55, in <module>` + "\nAssertionError: " + secret, "AssertionError", 0},
		{"foreign_path_suffix", trace + frame + "55, in <module>\n\n" + trace + `  File "C:\other\bridge.py", line 55, in <module>` + "\nRuntimeError: " + secret, "RuntimeError", 0},
		{"unknown_class", trace + frame + "55, in <module>\nPrivateException: " + secret, "unknown", 55},
		{"overlarge_line", trace + frame + "999999999999999999999999999999, in <module>\nAssertionError: " + secret, "AssertionError", 0},
		{"outside_source", trace + frame + "999999, in <module>\nAssertionError: " + secret, "AssertionError", 0},
		{"invalid_line", trace + frame + "55x, in <module>\nAssertionError: " + secret, "AssertionError", 0},
		{"negative_line", trace + frame + "-1, in <module>\nAssertionError: " + secret, "AssertionError", 0},
		{"arbitrary_body", secret + ` C:\private\document.docx`, "unknown", 0},
		{"oversize_stderr", strings.Repeat(secret, 4096) + "\n" + frame + "55, in <module>\nAssertionError", "unknown", 0},
	}
	for _, c := range cases {
		exception, line := pythonQualificationFailureFacts([]byte(c.stderr), bridge, documentWindowsBridge)
		if exception != c.exception || line != c.line {
			t.Errorf("%s: finite qualification facts got (%q, %d), want (%q, %d)", c.name, exception, line, c.exception, c.line)
		}
		facts, err := json.Marshal(struct {
			Exception string `json:"python_exception"`
			Line      int    `json:"python_bridge_line"`
		}{exception, line})
		if err != nil || strings.Contains(string(facts), secret) || strings.Contains(string(facts), "private") || strings.Contains(string(facts), "bridge.py") || strings.Contains(string(facts), "PrivateException") {
			t.Errorf("%s: qualification facts leaked private input", c.name)
		}
	}
}
