package windowsadapter

import "testing"

func TestTaskRenderIsParseableAndEnabledOnlyAsAdapterContract(t *testing.T) {
	body, err := Render(Spec{Name: "BCGOS-Darwin-Presence", Program: `C:\\bcgos.exe`, Arguments: "maintenance wake --trigger presence", WorkingDirectory: `C:\\workspace`, StartBoundary: "2026-08-01T10:00:00-03:00"})
	if err != nil || len(body) == 0 {
		t.Fatalf("body=%q err=%v", body, err)
	}
}

func TestTaskNativeLifecycleFailsClosedUntilQualified(t *testing.T) {
	status, err := Install(Spec{Name: "BCGOS-Darwin-Presence"}, true)
	if err != ErrNativeUnavailable || status.State != "unavailable_native_qualification_pending" {
		t.Fatalf("status=%#v err=%v", status, err)
	}
}
