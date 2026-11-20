//go:build windows

package docling

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// Called by the existing native-confinement parent; this adds no proof parent.
func assertWindowsQualificationFailureFactsPrivacy(t *testing.T) {
	t.Helper()
	const snapshot = `C:\private\snapshot`
	const secret = "PRIVATE_QUALIFICATION_MARKER"
	bridge := filepath.Join(snapshot, "bridge.py")
	module := filepath.Join(snapshot, "python", "Lib", "public_runtime.py")
	pack := verifiedPack{entries: map[string]entry{
		"bridge.py":                     {body: documentWindowsBridge},
		"python/Lib/empty.py":           {},
		"python/Lib/native.pyd":         {body: []byte("not source")},
		"python/Lib/public_runtime.py":  {body: []byte("first\nsecond\nlast")},
		"python/Lib/public_runtime.pyc": {body: []byte("compiled")},
		"python/Lib/z_binary.py":        {body: []byte("not\x00source")},
		"python/python312.zip":          {body: []byte("opaque")},
	}}
	pack.target.bridge = documentWindowsBridge
	trace := "Traceback (most recent call last):\n"
	frame := func(path, line string) string { return "  File \"" + path + "\", line " + line + ", in <module>\n" }
	want := func(exception string, bridge, ordinal, line, errno, winerror int) pythonFailureHints {
		return pythonFailureHints{exception, bridge, ordinal, line, errno, winerror}
	}
	cases := []struct {
		name, stderr string
		want         pythonFailureHints
	}{
		{"exact_bridge", trace + frame(bridge, "55") + "AssertionError: " + secret, want("AssertionError", 55, 1, 55, 0, 0)},
		{"public_innermost", trace + frame(bridge, "97") + frame(module, "3") + "PermissionError: " + secret, want("PermissionError", 97, 4, 3, 0, 0)},
		{"chained_exception", trace + frame(module, "1") + "PermissionError: [Errno 13] " + secret + "\n\nDuring handling of the above exception, another exception occurred:\n\n" + trace + frame(bridge, "55") + "AssertionError: " + secret, want("AssertionError", 55, 1, 55, 0, 0)},
		{"foreign_final_chain", trace + frame(module, "1") + "PermissionError: [WinError 5] " + secret + "\n\nThe above exception was the direct cause of the following exception:\n\n" + trace + frame(`C:\other\bridge.py`, "55") + "RuntimeError: " + secret, want("RuntimeError", 0, 0, 0, 0, 0)},
		{"foreign_innermost", trace + frame(bridge, "97") + frame(`C:\other\public_runtime.py`, "2") + "PermissionError: " + secret, want("PermissionError", 97, 0, 0, 0, 0)},
		{"relative", trace + frame(`python\Lib\public_runtime.py`, "2") + "PermissionError: " + secret, want("PermissionError", 0, 0, 0, 0, 0)},
		{"near_root", trace + frame(`C:\private\snapshot-evil\python\Lib\public_runtime.py`, "2") + "PermissionError: " + secret, want("PermissionError", 0, 0, 0, 0, 0)},
		{"near_name", trace + frame(module+".secret", "2") + "PermissionError: " + secret, want("PermissionError", 0, 0, 0, 0, 0)},
		{"native", trace + frame(module, "2") + frame(filepath.Join(snapshot, "python", "Lib", "native.pyd"), "1") + "PermissionError: " + secret, want("PermissionError", 0, 0, 0, 0, 0)},
		{"frozen", trace + frame(module, "2") + frame("<frozen importlib._bootstrap>", "1") + "PermissionError: " + secret, want("PermissionError", 0, 0, 0, 0, 0)},
		{"compiled", trace + frame(filepath.Join(snapshot, "python", "Lib", "public_runtime.pyc"), "1") + "PermissionError: " + secret, want("PermissionError", 0, 0, 0, 0, 0)},
		{"zip", trace + frame(filepath.Join(snapshot, "python", "python312.zip", "public_runtime.py"), "1") + "PermissionError: " + secret, want("PermissionError", 0, 0, 0, 0, 0)},
		{"empty_source", trace + frame(filepath.Join(snapshot, "python", "Lib", "empty.py"), "1") + "PermissionError: " + secret, want("PermissionError", 0, 0, 0, 0, 0)},
		{"malformed_innermost", trace + frame(module, "2") + "  File \"" + module + "\", line 1\nPermissionError: " + secret, want("PermissionError", 0, 0, 0, 0, 0)},
		{"tab_delimited_foreign_innermost", trace + frame(module, "2") + "  File\t\"C:\\other\\foreign.py\", line 1, in <module>\nPermissionError: " + secret, want("PermissionError", 0, 0, 0, 0, 0)},
		{"wrong_indent_innermost", trace + frame(module, "2") + " File \"" + module + "\", line 1, in <module>\nPermissionError: " + secret, want("PermissionError", 0, 0, 0, 0, 0)},
		{"outside_source", trace + frame(module, "4") + "PermissionError: " + secret, want("PermissionError", 0, 0, 0, 0, 0)},
		{"overlarge_line", trace + frame(module, "999999999999999999999999999999") + "PermissionError: " + secret, want("PermissionError", 0, 0, 0, 0, 0)},
		{"noncanonical_line", trace + frame(module, "02") + "PermissionError: " + secret, want("PermissionError", 0, 0, 0, 0, 0)},
		{"negative_line", trace + frame(module, "-1") + "PermissionError: " + secret, want("PermissionError", 0, 0, 0, 0, 0)},
		{"overlarge_bridge_line", trace + frame(bridge, "999999999999999999999999999999") + "AssertionError: " + secret, want("AssertionError", 0, 0, 0, 0, 0)},
		{"outside_bridge_source", trace + frame(bridge, "999999") + "AssertionError: " + secret, want("AssertionError", 0, 0, 0, 0, 0)},
		{"negative_bridge_line", trace + frame(bridge, "-1") + "AssertionError: " + secret, want("AssertionError", 0, 0, 0, 0, 0)},
		{"binary_source", trace + frame(filepath.Join(snapshot, "python", "Lib", "z_binary.py"), "1") + "PermissionError: " + secret, want("PermissionError", 0, 0, 0, 0, 0)},
		{"invalid_bridge_line", trace + frame(bridge, "55x") + "AssertionError: " + secret, want("AssertionError", 0, 0, 0, 0, 0)},
		{"unknown_class", trace + frame(bridge, "55") + "PrivateException: " + secret, want("unknown", 55, 1, 55, 0, 0)},
		{"errno13", trace + frame(module, "2") + "PermissionError: [Errno 13] " + secret + ` C:\private\document.docx`, want("PermissionError", 0, 4, 2, 13, 0)},
		{"errno1", trace + frame(module, "2") + "PermissionError: [Errno 1] " + secret, want("PermissionError", 0, 4, 2, 1, 0)},
		{"winerror5", trace + frame(module, "2") + "PermissionError: [WinError 5] " + secret, want("PermissionError", 0, 4, 2, 0, 5)},
		{"numeric_other_class", trace + frame(module, "2") + "AssertionError: [Errno 13] " + secret, want("AssertionError", 0, 4, 2, 0, 0)},
		{"numeric_message_only", trace + frame(module, "2") + "PermissionError: " + secret + " [WinError 5]", want("PermissionError", 0, 4, 2, 0, 0)},
		{"numeric_undelimited", trace + frame(module, "2") + "PermissionError: [WinError 5]" + secret, want("PermissionError", 0, 4, 2, 0, 0)},
		{"numeric_unknown", trace + frame(module, "2") + "PermissionError: [WinError 87] " + secret, want("PermissionError", 0, 4, 2, 0, 0)},
		{"numeric_leading_zero", trace + frame(module, "2") + "PermissionError: [WinError 05] " + secret, want("PermissionError", 0, 4, 2, 0, 0)},
		{"numeric_negative", trace + frame(module, "2") + "PermissionError: [Errno -13] " + secret, want("PermissionError", 0, 4, 2, 0, 0)},
		{"numeric_oversize", trace + frame(module, "2") + "PermissionError: [Errno 999999999999999999999] " + secret, want("PermissionError", 0, 4, 2, 0, 0)},
		{"path_in_message_only", trace + "PermissionError: " + secret + " " + module, want("PermissionError", 0, 0, 0, 0, 0)},
		{"arbitrary_body", secret + ` C:\private\document.docx`, want("unknown", 0, 0, 0, 0, 0)},
		{"oversize_stderr", strings.Repeat(secret, 4096) + "\n" + trace + frame(module, "2") + "PermissionError", want("unknown", 0, 0, 0, 0, 0)},
	}
	for _, c := range cases {
		got := pythonQualificationFailureFacts([]byte(c.stderr), snapshot, pack)
		if got != c.want {
			t.Errorf("%s: finite qualification hints got %+v, want %+v", c.name, got, c.want)
		}
		facts, err := json.Marshal(got)
		if err != nil || strings.Contains(string(facts), secret) || strings.Contains(string(facts), "private") || strings.Contains(string(facts), "bridge.py") || strings.Contains(string(facts), "public_runtime.py") || strings.Contains(string(facts), "PrivateException") {
			t.Errorf("%s: qualification hints leaked private input", c.name)
		}
		var fields map[string]any
		if json.Unmarshal(facts, &fields) != nil || len(fields) != 6 {
			t.Errorf("%s: qualification hint projection changed", c.name)
		}
	}
	pack.entries = nil
	if got := pythonQualificationFailureFacts([]byte(trace+frame(bridge, "55")+"AssertionError"), snapshot, pack); got != want("AssertionError", 55, 0, 0, 0, 0) {
		t.Error("missing authenticated inventory supplied a public source identity")
	}
	if got := pythonQualificationFailureFacts([]byte(trace+frame("bridge.py", "55")+"PermissionError"), "relative", pack); got != want("PermissionError", 0, 0, 0, 0, 0) {
		t.Error("relative snapshot supplied a public source identity")
	}
	pack.target.bridge = nil
	if got := pythonQualificationFailureFacts([]byte(trace+frame(bridge, "55")+"AssertionError"), snapshot, pack); got != want("unknown", 0, 0, 0, 0, 0) {
		t.Error("missing trusted bridge source supplied failure hints")
	}
}
