package windowsadapter

import (
	"context"
	"runtime"
	"strings"
	"testing"
)

func TestNativeWakeWindowsArgumentsRemainLiteral(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{[]string{""}, `""`},
		{[]string{`C:\Owner Name\`}, `"C:\Owner Name\\"`},
		{[]string{`a"b`}, `"a\"b"`},
		{[]string{"maintenance-wake", "--workspace", "C:\\Owner Name\\Case"}, `"maintenance-wake" "--workspace" "C:\Owner Name\Case"`},
	}
	for _, c := range cases {
		if got := QuoteWakeArguments(c.in); got != c.want {
			t.Fatalf("literal argv got %q want %q", got, c.want)
		}
	}
}

func TestNativeWakeRequiresExactOwnedIdentityAndOperation(t *testing.T) {
	s := NativeWakeSpec{Name: "MaestroMaintenance_" + strings.Repeat("a", 64), BindingSHA256: strings.Repeat("a", 64), Program: `C:\Developer\maestro-install.exe`, Arguments: `"maintenance-wake"`, WorkingDirectory: `C:\Developer\Case`}
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	other := s
	other.Name = "foreign"
	if other.Validate() == nil {
		t.Fatal("foreign task admitted")
	}
	other = s
	other.Arguments = "bad\ncommand"
	if other.Validate() == nil {
		t.Fatal("control character admitted")
	}
	if _, err := NativeWake(context.Background(), s, "force"); err == nil {
		t.Fatal("unknown operation admitted")
	}
	if runtime.GOOS != "windows" {
		if _, err := NativeWake(context.Background(), s, "inspect"); err == nil {
			t.Fatal("non-Windows host claimed native TaskScheduler readback")
		}
	}
}
