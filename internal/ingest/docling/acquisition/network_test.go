package acquisition

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"testing"
)

func TestPublicInputPolicyRejectsRedirectAndAddressEscapes(t *testing.T) {
	const origin = "https://github.com/astral-sh/python-build-standalone/releases/download/20261026/cpython.tar.gz"
	for _, target := range []string{"http://github.com/astral-sh/python-build-standalone/releases/download/x/a", "https://name:secret@github.com/astral-sh/python-build-standalone/releases/download/x/a", "https://127.0.0.1/a", "https://files.pythonhosted.org:8443/a", "https://release-assets.githubusercontent.com/a#fragment", "https://evil.example/a"} {
		if _, err := publicURL(target, origin); err == nil {
			t.Fatalf("unsafe upstream accepted: %s", target)
		}
	}
	for _, address := range []string{"127.0.0.1", "10.0.0.1", "169.254.169.254", "100.64.1.1", "::1", "::ffff:127.0.0.1", "fc00::1", "fe80::1", "0.0.0.0", "224.0.0.1", "198.18.0.1"} {
		if publicAddress(netip.MustParseAddr(address)) {
			t.Fatalf("nonpublic address accepted: %s", address)
		}
	}
	for _, address := range []string{"140.82.112.3", "151.101.0.223", "2606:50c0:8000::154"} {
		if !publicAddress(netip.MustParseAddr(address)) {
			t.Fatalf("public address rejected: %s", address)
		}
	}
	if _, err := publicURL("https://release-assets.githubusercontent.com/github-production-release-asset/file?signature=public-asset-token", origin); err != nil {
		t.Fatal(err)
	}
	if _, err := publicURL("https://release-assets.githubusercontent.com/file", "https://files.pythonhosted.org/packages/pinned.whl"); err == nil {
		t.Fatal("PyPI redirect escaped its publisher host")
	}
}

func TestRedirectPolicyDropsHeadersAndBoundsHops(t *testing.T) {
	const origin = "https://github.com/astral-sh/python-build-standalone/releases/download/20261026/cpython.tar.gz"
	client := publicClient(origin)
	prior, _ := http.NewRequest("GET", origin, nil)
	next, _ := http.NewRequest("GET", "https://release-assets.githubusercontent.com/file?signature=asset-token", nil)
	next.Header.Set("Authorization", "secret")
	next.Header.Set("Cookie", "secret")
	next.Header.Set("Referer", origin+"?secret")
	if err := client.CheckRedirect(next, []*http.Request{prior}); err != nil {
		t.Fatal(err)
	}
	if next.Header.Get("Authorization") != "" || next.Header.Get("Cookie") != "" || next.Header.Get("Referer") != "" {
		t.Fatal("sensitive headers survived redirect")
	}
	if err := client.CheckRedirect(next, []*http.Request{prior, prior, prior, prior}); err == nil {
		t.Fatal("unbounded redirect chain accepted")
	}
}

func TestDNSIsVettedAndTheSocketUsesOnlyThePinnedNumericAddress(t *testing.T) {
	for _, addresses := range [][]string{{"127.0.0.1"}, {"140.82.112.3", "10.0.0.1"}, {"64:ff9b::7f00:1"}, {"140.82.112.3"}} {
		calls := 0
		resolved := 0
		resolver := func(_ context.Context, network, host string) ([]netip.Addr, error) {
			resolved++
			if host != "github.com" || network != "ip" {
				t.Fatal("wrong resolution")
			}
			var result []netip.Addr
			for _, a := range addresses {
				result = append(result, netip.MustParseAddr(a))
			}
			return result, nil
		}
		dial := func(_ context.Context, network, address string) (net.Conn, error) {
			calls++
			if address != "140.82.112.3:443" {
				t.Fatal("hostname was re-resolved or wrong address dialed")
			}
			return nil, errors.New("public socket fixture")
		}
		_, err := dialPublic(context.Background(), "tcp", "github.com:443", resolver, dial)
		if err == nil {
			t.Fatal("fixture accepted no connection")
		}
		expected := 0
		if len(addresses) == 1 && addresses[0] == "140.82.112.3" {
			expected = 1
		}
		if calls != expected || resolved != 1 {
			t.Fatal("DNS deny/pinning boundary failed")
		}
	}
}
