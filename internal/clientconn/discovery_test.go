package clientconn

import (
	"path/filepath"
	"testing"
)

func TestResolveFilePathPrefersExplicitOverride(t *testing.T) {
	explicit := filepath.Join(t.TempDir(), "explicit.json")
	environment := map[string]string{
		"MODA_CLIENT_MCP_CONNECTION_FILE": filepath.Join(t.TempDir(), "environment.json"),
	}

	got, err := ResolveFilePath(explicit, environment, "/home/test", "linux")
	if err != nil {
		t.Fatalf("ResolveFilePath() error = %v", err)
	}
	if got != explicit {
		t.Fatalf("ResolveFilePath() = %q, want %q", got, explicit)
	}
}

func TestResolveFilePathUsesPlatformDefaults(t *testing.T) {
	tests := []struct {
		platform string
		env      map[string]string
		want     string
	}{
		{
			platform: "darwin",
			want:     filepath.Join("/home/test", "Library", "Application Support", "Moda", FileName),
		},
		{
			platform: "windows",
			env:      map[string]string{"APPDATA": filepath.Join("C:", "Users", "test", "AppData", "Roaming")},
			want:     filepath.Join("C:", "Users", "test", "AppData", "Roaming", "Moda", FileName),
		},
		{
			platform: "linux",
			env:      map[string]string{"XDG_CONFIG_HOME": "/config"},
			want:     filepath.Join("/config", "Moda", FileName),
		},
	}
	for _, test := range tests {
		t.Run(test.platform, func(t *testing.T) {
			got, err := ResolveFilePath("", test.env, "/home/test", test.platform)
			if err != nil {
				t.Fatalf("ResolveFilePath() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("ResolveFilePath() = %q, want %q", got, test.want)
			}
		})
	}
}
