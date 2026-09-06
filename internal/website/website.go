// Package website serves Termon's public landing page without exposing operator endpoints.
package website

import (
	"bytes"
	"embed"
	"encoding/json"
	"html/template"
	"io/fs"
	"net/http"
	"strings"

	"golang.org/x/crypto/ssh"
)

//go:embed static/*
var assets embed.FS

// New renders the connection instructions from the SSH server's actual host key.
// online must be safe to call concurrently with game sessions.
func New(key ssh.PublicKey, online func() int) (http.Handler, error) {
	page, err := template.ParseFS(assets, "static/index.html")
	if err != nil {
		return nil, err
	}
	var rendered bytes.Buffer
	err = page.Execute(&rendered, struct {
		Fingerprint string
		Command     string
	}{
		Fingerprint: ssh.FingerprintSHA256(key),
		Command: "key_dir=$(mktemp -d) &&\n" +
			"ssh-keygen -q -t ed25519 -N '' -C termon -f \"$key_dir/key\" &&\n" +
			"printf '%s\\n' 'termon.sh " + strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key))) +
			"' > \"$key_dir/known_hosts\" &&\n" +
			"printf 'Keep this key to return to your Trainer: %s\\n' \"$key_dir/key\" &&\n" +
			"ssh -F /dev/null -i \"$key_dir/key\" -o IdentitiesOnly=yes " +
			"-o StrictHostKeyChecking=yes -o UserKnownHostsFile=\"$key_dir/known_hosts\" termon.sh",
	})
	if err != nil {
		return nil, err
	}
	public, err := fs.Sub(assets, "static")
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(rendered.Bytes())
	})
	mux.HandleFunc("GET /api/online", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(struct {
			Online int `json:"online"`
		}{Online: online()})
	})
	for _, name := range []string{"style.css", "app.js", "demo.gif", "demo.png"} {
		mux.Handle("GET /"+name, http.FileServerFS(public))
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		mux.ServeHTTP(w, r)
	}), nil
}
