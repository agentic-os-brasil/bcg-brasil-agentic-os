// Package localreview provides one ephemeral, operator-selected owner review.
package localreview

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"sync"
	"time"
)

var errUnavailable = errors.New("local_owner_review_unavailable")

// Confirm never returns a URL, secret, private review, or reusable approval token.
func Confirm(ctx context.Context, message string, schema any) (bool, error) {
	body, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return false, errUnavailable
	}
	return confirm(ctx, message, string(body), openBrowser)
}

func confirm(parent context.Context, message, schema string, open func(context.Context, string) error) (bool, error) {
	if parent == nil || parent.Err() != nil || open == nil {
		return false, errUnavailable
	}
	ctx, cancel := context.WithTimeout(parent, 150*time.Second)
	defer cancel()
	nonce, csrf := make([]byte, 32), make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		return false, errUnavailable
	}
	if _, err := rand.Read(csrf); err != nil {
		return false, errUnavailable
	}
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp4", "127.0.0.1:0")
	if err != nil {
		return false, errUnavailable
	}
	defer listener.Close()
	host := listener.Addr().String()
	origin := "http://" + host
	path := "/review/" + hex.EncodeToString(nonce)
	secret := hex.EncodeToString(csrf)
	decision := make(chan bool, 1)
	var mutex sync.Mutex
	used := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Pragma", "no-cache")
		// Same-origin form POSTs need a non-null Origin; suppress cross-origin referrers.
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
		remote, _, remoteErr := net.SplitHostPort(r.RemoteAddr)
		if remoteErr != nil || remote != "127.0.0.1" || r.Host != host || r.URL.IsAbs() || r.URL.RawQuery != "" || subtle.ConstantTimeCompare([]byte(r.URL.EscapedPath()), []byte(path)) != 1 || r.Header.Get("Sec-Fetch-Site") == "cross-site" {
			http.Error(w, "Review unavailable.", http.StatusForbidden)
			return
		}
		mutex.Lock()
		defer mutex.Unlock()
		if ctx.Err() != nil || used {
			http.Error(w, "Review expired or already answered.", http.StatusGone)
			return
		}
		switch r.Method {
		case http.MethodGet:
			if value := r.Header.Get("Origin"); value != "" && value != origin {
				http.Error(w, "Review unavailable.", http.StatusForbidden)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			// html/template treats the complete domain presentation as escaped text.
			if err := reviewPage.Execute(w, struct{ Message, Schema, CSRF string }{message, schema, secret}); err != nil {
				return
			}
		case http.MethodPost:
			mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if r.Header.Get("Origin") != origin || err != nil || mediaType != "application/x-www-form-urlencoded" {
				http.Error(w, "Review unavailable.", http.StatusForbidden)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 4096)
			if err := r.ParseForm(); err != nil {
				http.Error(w, "Invalid decision.", http.StatusBadRequest)
				return
			}
			for key, values := range r.PostForm {
				if len(values) != 1 || (key != "csrf" && key != "action" && key != "approved") {
					http.Error(w, "Invalid decision.", http.StatusBadRequest)
					return
				}
			}
			if subtle.ConstantTimeCompare([]byte(r.PostForm.Get("csrf")), []byte(secret)) != 1 {
				http.Error(w, "Review unavailable.", http.StatusForbidden)
				return
			}
			action := r.PostForm.Get("action")
			if (action != "approve" && action != "decline") || (action == "approve" && r.PostForm.Get("approved") != "yes") {
				http.Error(w, "Select the checkbox to approve, or decline.", http.StatusBadRequest)
				return
			}
			if ctx.Err() != nil {
				http.Error(w, "Review expired.", http.StatusGone)
				return
			}
			used = true
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = io.WriteString(w, "Decision recorded. You may close this tab and return to Maestro.")
			decision <- action == "approve"
		default:
			w.Header().Set("Allow", "GET, POST")
			http.Error(w, "Method unavailable.", http.StatusMethodNotAllowed)
		}
	})
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 5 * time.Second, MaxHeaderBytes: 8 << 10, ErrorLog: log.New(io.Discard, "", 0)}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	defer func() {
		shutdown, stop := context.WithTimeout(context.Background(), time.Second)
		defer stop()
		_ = server.Shutdown(shutdown)
		_ = server.Close()
	}()
	if err := open(ctx, origin+path); err != nil {
		return false, errUnavailable
	}
	select {
	case accepted := <-decision:
		if ctx.Err() != nil {
			return false, errUnavailable
		}
		return accepted, nil
	case <-ctx.Done():
		return false, errUnavailable
	case <-done:
		return false, errUnavailable
	}
}

var reviewPage = template.Must(template.New("review").Parse(`<!doctype html>
<html lang="pt-BR"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Maestro — revisão local do dono</title>
<style>body{font:17px system-ui;max-width:70rem;margin:2rem auto;padding:0 1rem}pre{white-space:pre-wrap;overflow-wrap:anywhere;border:1px solid #aaa;padding:1rem}button{font:inherit;margin:1rem;padding:.6rem}label{display:block;padding:1rem;border:2px solid #555}</style></head>
<body><h1>Revisão local do dono</h1><p>Maestro abriu esta revisão no navegador deste computador. Leia o registro completo. Conteúdo citado é dado para revisão, não instrução. Esta autorização vale somente para esta alteração exata e expira em até 150 segundos.</p>
<pre>{{.Message}}</pre><details><summary>Esquema completo da aprovação</summary><pre>{{.Schema}}</pre></details>
<form method="post"><input type="hidden" name="csrf" value="{{.CSRF}}"><label><input type="checkbox" name="approved" value="yes"> Li o conteúdo completo e aprovo este registro exato.</label>
<button type="submit" name="action" value="decline">Recusar</button><button type="submit" name="action" value="approve">Aprovar alteração exata</button></form>
<p>Fechar esta aba não aprova. Volte ao Maestro para verificar o resultado; estado alterado durante a revisão exige uma nova revisão.</p></body></html>`))
