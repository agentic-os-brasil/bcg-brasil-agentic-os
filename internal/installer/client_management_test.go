package installer

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFixedUpdateLauncherKeepsNativeReviewWithExactParticipantSelection(t *testing.T) {
	o := ClientOptions{SeededOperationOptions: SeededOperationOptions{Self: "/Users/participant/Developer/Maestro/maestro-install", ManagedRoot: "/Users/participant/Developer/Maestro", UserHome: "/Users/participant"}, Request: ClientRequest{CarrierID: strings.Repeat("a", 32), Workspace: "/Users/participant/Developer/Brain", CodeRoot: "/Users/participant/Developer/Clients/Code", DesktopPath: "/Users/participant/Developer/Clients/Desktop.mcpb", CodeScope: "user"}}
	_, body := ClientManagementLauncher(o, true)
	if !bytes.Contains(body, []byte("--management-intent")) || !bytes.Contains(body, []byte("update")) || bytes.Contains(body, []byte("--confirm")) || bytes.Contains(body, []byte("--review-digest")) {
		t.Fatal("handoff recipe altered installer approval boundary")
	}
	root := t.TempDir()
	path := filepath.Join(root, "Update Maestro.command")
	if err := os.WriteFile(path, body, 0700); err != nil {
		t.Fatal(err)
	}
	if err := verifyManagementFile(path, body, true); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("foreign command\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := verifyManagementFile(path, body, true); err == nil {
		t.Fatal("changed update launcher admitted")
	}
	if bytes.Contains(bytes.ToLower(ClientWindowsManagementEntry()), []byte("bypass")) {
		t.Fatal("Windows normal policy bypassed")
	}
}
