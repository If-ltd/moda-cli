package direct

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestSessionStoreSavesPrivateSessionWithoutLoginCredentials(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "session.json")
	store := NewSessionStore(filePath)
	want := Session{
		APIBaseURL:   "https://api.moda.test",
		AccessToken:  "access-1",
		RefreshToken: "refresh-1",
		ExpiresAt:    time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	if err := store.Save(want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got != want {
		t.Fatalf("Load() = %+v, want %+v", got, want)
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "password") || strings.Contains(string(data), "account") {
		t.Fatalf("session contains login credentials: %s", data)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filePath)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("session permissions = %o, want 600", info.Mode().Perm())
		}
	}
}

func TestResolveSessionFilePathUsesExplicitOverride(t *testing.T) {
	explicit := filepath.Join(t.TempDir(), "direct.json")
	got, err := ResolveSessionFilePath(explicit, map[string]string{
		"MODA_CLIENT_CLI_SESSION_FILE": filepath.Join(t.TempDir(), "environment.json"),
	}, "/home/test", "linux")
	if err != nil {
		t.Fatalf("ResolveSessionFilePath() error = %v", err)
	}
	if got != explicit {
		t.Fatalf("ResolveSessionFilePath() = %q, want %q", got, explicit)
	}
}
