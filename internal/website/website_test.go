package website

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"html"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestPublicWebsite(t *testing.T) {
	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ssh.NewPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	count := 3
	handler, err := New(key, func() int { return count })
	if err != nil {
		t.Fatal(err)
	}
	get := func(path string) *httptest.ResponseRecorder {
		t.Helper()
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		return response
	}
	page := get("/")
	if page.Code != http.StatusOK {
		t.Fatalf("page returned %d", page.Code)
	}
	for _, want := range []string{
		ssh.FingerprintSHA256(key), strings.Fields(string(ssh.MarshalAuthorizedKey(key)))[1],
		"IdentitiesOnly=yes", "StrictHostKeyChecking=yes", "ssh-keygen", "IPs may appear in server logs",
	} {
		if !strings.Contains(html.UnescapeString(page.Body.String()), want) {
			t.Errorf("page missing %q", want)
		}
	}
	for _, expected := range []int{3, 0, 12} {
		count = expected
		response := get("/api/online")
		var data struct {
			Online int `json:"online"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &data); err != nil {
			t.Fatal(err)
		}
		if data.Online != expected || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("count response = %s, headers = %v", response.Body, response.Header())
		}
	}
	for _, path := range []string{"/style.css", "/app.js", "/demo.gif", "/demo.png"} {
		if response := get(path); response.Code != http.StatusOK || response.Body.Len() == 0 {
			t.Errorf("asset %s returned %d with %d bytes", path, response.Code, response.Body.Len())
		}
	}
	for _, path := range []string{"/metrics", "/readyz", "/debug/pprof/", "/static/index.html", "/host-key"} {
		if response := get(path); response.Code != http.StatusNotFound {
			t.Errorf("private path %s returned %d", path, response.Code)
		}
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/online", nil))
	if response.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST returned %d", response.Code)
	}
}

func TestFingerprintMatchesOpenSSH(t *testing.T) {
	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		t.Skip("OpenSSH is not installed")
	}
	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ssh.NewPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "host.pub")
	if err := os.WriteFile(path, ssh.MarshalAuthorizedKey(key), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.CommandContext(t.Context(), "ssh-keygen", "-lf", path, "-E", "sha256").Output()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Fields(string(output))[1]; got != ssh.FingerprintSHA256(key) {
		t.Fatalf("ssh-keygen fingerprint %s differs from website fingerprint %s", got, ssh.FingerprintSHA256(key))
	}
}
