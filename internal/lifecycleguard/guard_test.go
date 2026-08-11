package lifecycleguard

import "testing"

func TestProtectedRootRemovalCoversSupportedCommandForms(t *testing.T) {
	tests := map[string]bool{
		"rm -rf /":                    true,
		"rm -R -f /":                  true,
		"rm --recursive --force /":    true,
		"rm -rf -- /":                 true,
		"rm -rf /tmp /":               true,
		"rm -rf //":                   true,
		"FOO=bar rm -rf /":            true,
		"PATH=/tmp HOME=/x rm -rf /":  true,
		"rm -rf \"/\"":                true,
		"rm -rf ~":                    true,
		"rm -rf $HOME":                true,
		"rm -rf ${HOME}":              true,
		"rm --preserve-root -rf /":    true,
		"rm -rf /tmp":                 false,
		"rm -rf ~/project":            false,
		"rm -rf \"$HOME/project\"":    false,
		"rm -rf '$HOME'":              false,
		"rm -r /":                     false,
		"rm -f /":                     false,
		"rm -rf":                      false,
		"sudo rm -rf /":               false,
		"env FOO=bar rm -rf /":        false,
		"sh -c 'rm -rf /'":            false,
		"rm -rf ./":                   false,
		"rm -rf $HOME/project":        false,
		"rm -rf ${HOME}/project":      false,
		"rm -rf /workspace/client":    false,
		"rm -rf -- /workspace/client": false,
	}
	for command, want := range tests {
		t.Run(command, func(t *testing.T) {
			got, err := ProtectedRootRemoval(command)
			if err != nil {
				t.Fatalf("ProtectedRootRemoval(%q) error = %v", command, err)
			}
			if got != want {
				t.Fatalf("ProtectedRootRemoval(%q) = %v, want %v", command, got, want)
			}
		})
	}
}

func TestProtectedRootRemovalRejectsCommandsOutsideBoundedGrammar(t *testing.T) {
	if destructive, err := ProtectedRootRemoval("rm -rf / && echo unsafe"); err != nil || !destructive {
		t.Fatalf("protected-root chain must be denied deterministically: destructive=%v err=%v", destructive, err)
	}
	for _, command := range []string{
		"rm -rf /; echo unsafe",
		"rm -rf /\nwhoami",
		"rm -rf / | cat",
		"rm -rf `pwd`",
		"rm -rf *",
		"rm -rf $USER",
		"rm -rf ${USER}",
		"rm -rf \"$HOME",
		"rm -rf /\\",
	} {
		t.Run(command, func(t *testing.T) {
			if _, err := ProtectedRootRemoval(command); err == nil {
				t.Fatalf("ProtectedRootRemoval(%q) accepted an unsupported command", command)
			}
		})
	}
}

func TestProtectedRootRemovalKeepsWrapperAndNonRootTargetsSafe(t *testing.T) {
	for _, command := range []string{
		"sudo -- rm -rf /",
		"env -- rm -rf /",
		"command rm -rf /",
		"rm -rf \"$HOME/project\" /tmp",
		"rm -rf /home/../workspace",
	} {
		t.Run(command, func(t *testing.T) {
			got, err := ProtectedRootRemoval(command)
			if err != nil {
				t.Fatalf("ProtectedRootRemoval(%q) error = %v", command, err)
			}
			if got {
				t.Fatalf("ProtectedRootRemoval(%q) = true for a non-root or wrapped command", command)
			}
		})
	}
}

func TestProtectedRootRemovalAllowsBoundedSafeAndThenButStillProtectsRoots(t *testing.T) {
	tests := []struct {
		command     string
		destructive bool
	}{
		{command: "rm canary/sandbox/nota.md && echo ok", destructive: false},
		{command: "rm -- canary/sandbox/nota.md && printf done", destructive: false},
		{command: "rm -rf / && echo unsafe", destructive: true},
		{command: "echo begin && /bin/rm -Rf $HOME", destructive: true},
	}
	for _, test := range tests {
		got, err := ProtectedRootRemoval(test.command)
		if err != nil {
			t.Fatalf("ProtectedRootRemoval(%q) error = %v", test.command, err)
		}
		if got != test.destructive {
			t.Fatalf("ProtectedRootRemoval(%q) = %v, want %v", test.command, got, test.destructive)
		}
	}
}

func TestLeadingAssignmentValidation(t *testing.T) {
	for _, test := range []struct {
		value string
		want  bool
	}{
		{value: "FOO=bar", want: true},
		{value: "_FOO=bar", want: true},
		{value: "F2=bar", want: true},
		{value: "2F=bar", want: false},
		{value: "FOO-BAR=bar", want: false},
		{value: "FOO", want: false},
	} {
		t.Run(test.value, func(t *testing.T) {
			if got := isLeadingAssignment(test.value); got != test.want {
				t.Fatalf("isLeadingAssignment(%q) = %v, want %v", test.value, got, test.want)
			}
		})
	}
}
