package localreview

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestExactReviewAndConsciousApproval(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	approved, err := confirm(ctx, "Exact <script>private</script>\n"+strings.Repeat("full body ", 1000), `{ "approved": { "type": "boolean" } }`, func(_ context.Context, address string) error {
		response, err := http.Get(address)
		if err != nil {
			return err
		}
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		text := string(body)
		if response.StatusCode != 200 || !strings.Contains(text, "&lt;script&gt;private&lt;/script&gt;") || !strings.Contains(text, strings.Repeat("full body ", 1000)) || strings.Contains(text, "checked") {
			t.Errorf("full escaped review/unchecked checkbox missing: status %d", response.StatusCode)
		}
		for _, header := range []string{"Content-Security-Policy", "Cache-Control", "Referrer-Policy", "X-Frame-Options"} {
			if response.Header.Get(header) == "" {
				t.Errorf("missing %s", header)
			}
		}
		csrf := regexp.MustCompile(`name="csrf" value="([a-f0-9]+)"`).FindStringSubmatch(text)
		if len(csrf) != 2 {
			t.Fatal("missing CSRF field")
		}
		u, _ := url.Parse(address)
		values := url.Values{"csrf": {csrf[1]}, "action": {"approve"}, "approved": {"yes"}}
		request, _ := http.NewRequest(http.MethodPost, address, strings.NewReader(values.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		origin := u.Scheme + "://" + u.Host
		// Fetch serializes a non-CORS POST's Origin as null under no-referrer.
		if response.Header.Get("Referrer-Policy") == "no-referrer" {
			origin = "null"
		}
		request.Header.Set("Origin", origin)
		result, err := http.DefaultClient.Do(request)
		if err == nil {
			io.Copy(io.Discard, result.Body)
			result.Body.Close()
			if result.StatusCode != 200 {
				t.Errorf("approval status %d", result.StatusCode)
			}
		}
		return err
	})
	if err != nil || !approved {
		t.Fatalf("approval: %v, %v", approved, err)
	}
}

func reviewForm(address string) (url.Values, error) {
	response, err := http.Get(address)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	csrf := regexp.MustCompile(`name="csrf" value="([a-f0-9]+)"`).FindStringSubmatch(string(body))
	if len(csrf) != 2 {
		return nil, fmt.Errorf("review unavailable: %d", response.StatusCode)
	}
	return url.Values{"csrf": {csrf[1]}, "action": {"approve"}, "approved": {"yes"}}, nil
}
func submit(address string, values url.Values, change func(*http.Request)) (int, error) {
	request, err := http.NewRequest(http.MethodPost, address, strings.NewReader(values.Encode()))
	if err != nil {
		return 0, err
	}
	u, _ := url.Parse(address)
	request.Header.Set("Origin", u.Scheme+"://"+u.Host)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if change != nil {
		change(request)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	_, err = io.Copy(io.Discard, response.Body)
	return response.StatusCode, err
}
func TestDecisionGuardsAndDecline(t *testing.T) {
	cases := []struct {
		name    string
		values  func(url.Values)
		request func(*http.Request)
	}{
		{name: "wrong_host", request: func(r *http.Request) { r.Host = "attacker.invalid" }},
		{name: "wrong_origin", request: func(r *http.Request) { r.Header.Set("Origin", "http://attacker.invalid") }},
		{name: "null_origin", request: func(r *http.Request) { r.Header.Set("Origin", "null") }},
		{name: "missing_origin", request: func(r *http.Request) { r.Header.Del("Origin") }},
		{name: "wrong_nonce", request: func(r *http.Request) { r.URL.Path = "/review/" + strings.Repeat("0", 64) }},
		{name: "path_traversal", request: func(r *http.Request) { r.URL.Path = "/review/../foreign" }},
		{name: "query", request: func(r *http.Request) { r.URL.RawQuery = "csrf=unexpected" }},
		{name: "cross_site", request: func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "cross-site") }},
		{name: "wrong_csrf", values: func(v url.Values) { v.Set("csrf", strings.Repeat("0", 64)) }},
		{name: "unchecked", values: func(v url.Values) { v.Del("approved") }},
		{name: "wrong_checkbox", values: func(v url.Values) { v.Set("approved", "true") }},
		{name: "duplicate", values: func(v url.Values) { v.Add("action", "decline") }},
		{name: "unknown_field", values: func(v url.Values) { v.Set("model_approved", "true") }},
		{name: "oversize", values: func(v url.Values) { v.Set("csrf", strings.Repeat("x", 5000)) }},
		{name: "wrong_type", request: func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			approved, err := confirm(ctx, "exact private record", "schema", func(_ context.Context, address string) error {
				values, err := reviewForm(address)
				if err != nil {
					return err
				}
				valid := url.Values{}
				for k, v := range values {
					valid[k] = append([]string(nil), v...)
				}
				if tc.values != nil {
					tc.values(values)
				}
				status, err := submit(address, values, tc.request)
				if err != nil || status < 400 {
					t.Errorf("invalid decision accepted: %d %v", status, err)
				}
				valid.Set("action", "decline")
				valid.Del("approved")
				status, err = submit(address, valid, nil)
				if err == nil && status != 200 {
					t.Errorf("decline status %d", status)
				}
				return err
			})
			if err != nil || approved {
				t.Fatalf("invalid/decline granted approval: %v %v", approved, err)
			}
		})
	}
}
func TestSingleUseApproval(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	approved, err := confirm(ctx, "exact record", "schema", func(_ context.Context, address string) error {
		values, err := reviewForm(address)
		if err != nil {
			return err
		}
		status, err := submit(address, values, nil)
		if err != nil {
			return err
		}
		if status != 200 {
			t.Errorf("first status %d", status)
		}
		status, err = submit(address, values, nil)
		if err == nil && status != http.StatusGone {
			t.Errorf("replay status %d", status)
		}
		return err
	})
	if err != nil || !approved {
		t.Fatalf("decision %v %v", approved, err)
	}
}
func TestUnavailableNeverGrantsAuthorityAndClosesListener(t *testing.T) {
	for _, mode := range []string{"opener_error", "cancel", "expiry", "cancel_before_open", "nil_context", "no_opener"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			if mode == "cancel_before_open" {
				cancel()
			}
			var parent context.Context = ctx
			if mode == "nil_context" {
				parent = nil
			}
			address := ""
			opened := false
			opener := func(ctx context.Context, u string) error {
				opened = true
				address = u
				if mode == "opener_error" {
					return errors.New("private " + u)
				}
				if mode == "cancel" {
					cancel()
				}
				<-ctx.Done()
				return nil
			}
			if mode == "no_opener" {
				opener = nil
			}
			approved, err := confirm(parent, "private record", "schema", opener)
			if approved || err == nil || strings.Contains(err.Error(), "127.0.0.1") || strings.Contains(err.Error(), "private") {
				t.Fatalf("unexpected authority/error: %v %v", approved, err)
			}
			if (mode == "cancel_before_open" || mode == "nil_context" || mode == "no_opener") && opened {
				t.Fatal("unavailable request opened browser")
			}
			if address != "" {
				response, err := http.Get(address)
				if err == nil {
					response.Body.Close()
					t.Fatal("listener remained available after request ended")
				}
			}
		})
	}
}
