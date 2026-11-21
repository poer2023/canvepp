package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"
	"time"
)

// Use an isolated test key and a second process, as Credential Manager must
// restore a rotated token even after every in-memory client has gone away.
func TestPinterestWindowsCredentialRestart(t *testing.T) {
	dir := os.Getenv("CANVEPP_TEST_PINTEREST_REOPEN")
	if dir != "" {
		session, err := newPinterestVault(dir).load()
		if err != nil || session.Token.Refresh != "fixture-rotated" || session.Token.Access != "fixture-renewed" {
			t.Fatal("could not reopen persisted Pinterest authorization", err)
		}
		return
	}
	dir = t.TempDir()
	vault := newPinterestVault(dir)
	session := pinterestSession{Config: testPinterestConfig(), Token: pinterestToken{Access: "fixture-expired", Refresh: "fixture-before", ExpiresAt: time.Now().Add(-time.Hour).Unix()}}
	if err := vault.save(session); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := vault.store.Delete(vault.key); err != nil {
			t.Error(err)
		}
	})
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/token" || r.FormValue("refresh_token") != "fixture-before" {
			http.Error(w, "invalid", 400)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"access_token": "fixture-renewed", "refresh_token": "fixture-rotated", "expires_in": 3600, "token_type": "bearer"})
	}))
	defer mock.Close()
	client := newPinterestClient(session, vault)
	client.base = mock.URL
	if _, err := client.access(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	child := exec.Command(exe, "-test.run=^TestPinterestWindowsCredentialRestart$")
	child.Env = append(os.Environ(), "CANVEPP_TEST_PINTEREST_REOPEN="+dir)
	if out, err := child.CombinedOutput(); err != nil {
		t.Fatalf("restart: %v %s", err, out)
	}
	if err := client.Disconnect(); err != nil {
		t.Fatal(err)
	}
	saved, err := vault.load()
	if err != nil || saved.Token.Access != "" || saved.Token.Refresh != "" {
		t.Fatal("disconnect retained login", err)
	}
}
