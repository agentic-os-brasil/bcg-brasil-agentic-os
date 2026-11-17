package acquisition

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

func publicURL(raw, initial string) (*url.URL, error) {
	origin, err := url.Parse(initial)
	if err != nil || origin.Scheme != "https" || origin.User != nil || origin.RawQuery != "" || origin.Fragment != "" || origin.Opaque != "" || origin.Port() != "" {
		return nil, errors.New("acquisition_url_refused")
	}
	host := strings.ToLower(origin.Hostname())
	if host != "files.pythonhosted.org" && (host != "github.com" || !strings.HasPrefix(origin.Path, "/astral-sh/python-build-standalone/releases/download/")) {
		return nil, errors.New("acquisition_url_refused")
	}
	target, err := url.Parse(raw)
	if err != nil || target.Scheme != "https" || target.User != nil || target.Fragment != "" || target.Opaque != "" || (target.Port() != "" && target.Port() != "443") {
		return nil, errors.New("acquisition_url_refused")
	}
	targetHost := strings.ToLower(target.Hostname())
	if host == "files.pythonhosted.org" {
		if targetHost != host || target.RawQuery != "" {
			return nil, errors.New("acquisition_url_refused")
		}
	} else {
		if targetHost != "github.com" && targetHost != "release-assets.githubusercontent.com" && targetHost != "objects.githubusercontent.com" {
			return nil, errors.New("acquisition_url_refused")
		}
		if targetHost == "github.com" && (!strings.HasPrefix(target.Path, "/astral-sh/python-build-standalone/releases/download/") || target.RawQuery != "") {
			return nil, errors.New("acquisition_url_refused")
		}
	}
	return target, nil
}

func publicAddress(address netip.Addr) bool {
	address = address.Unmap()
	if !address.IsValid() || address.Zone() != "" || !address.IsGlobalUnicast() || address.IsPrivate() {
		return false
	}
	if address.Is6() && !netip.MustParsePrefix("2000::/3").Contains(address) {
		return false
	}
	for _, reserved := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001::/23", "2001:db8::/32", "2002::/16", "3fff::/20"} {
		if netip.MustParsePrefix(reserved).Contains(address) {
			return false
		}
	}
	return true
}

func publicClient(initial string) *http.Client {
	transport := &http.Transport{Proxy: nil, DisableCompression: true, DisableKeepAlives: true, MaxConnsPerHost: 1, TLSHandshakeTimeout: 30 * time.Second, ResponseHeaderTimeout: 30 * time.Second, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		return dialPublic(ctx, network, address, net.DefaultResolver.LookupNetIP, (&net.Dialer{Timeout: 30 * time.Second}).DialContext)
	}

	return &http.Client{Transport: transport, Timeout: 5 * time.Minute, CheckRedirect: func(request *http.Request, via []*http.Request) error {
		if len(via) >= 4 {
			return errors.New("acquisition_redirect_limit")
		}
		if _, err := publicURL(request.URL.String(), initial); err != nil {
			return err
		}
		request.Header = http.Header{"User-Agent": []string{"Maestro-Engineering-Runtime/1"}}
		return nil
	}}
}

func allowedDialHost(host string) bool {
	switch strings.ToLower(host) {
	case "github.com", "files.pythonhosted.org", "release-assets.githubusercontent.com", "objects.githubusercontent.com":
		return true
	}
	return false
}

// Resolution and socket creation stay in one operation: TLS retains the original
// HTTPS hostname while the socket receives only this vetted numeric address.
func dialPublic(ctx context.Context, network, address string, resolve func(context.Context, string, string) ([]netip.Addr, error), dial func(context.Context, string, string) (net.Conn, error)) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil || port != "443" || !allowedDialHost(host) {
		return nil, errors.New("acquisition_destination_refused")
	}
	addresses, err := resolve(ctx, "ip", host)
	if err != nil || len(addresses) == 0 {
		return nil, errors.New("acquisition_upstream_unavailable")
	}
	for _, candidate := range addresses {
		if !publicAddress(candidate) {
			return nil, errors.New("acquisition_destination_refused")
		}
	}
	connection, err := dial(ctx, network, net.JoinHostPort(addresses[0].String(), port))
	if err != nil {
		return nil, errors.New("acquisition_upstream_unavailable")
	}
	return connection, nil
}
