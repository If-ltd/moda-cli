package clientconn

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestReadAcceptsValidV1Descriptor(t *testing.T) {
	now := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	rootCAPath := filepath.Join(t.TempDir(), "root-ca.pem")
	if err := os.WriteFile(rootCAPath, []byte("test root CA"), 0o600); err != nil {
		t.Fatal(err)
	}
	filePath := writeDescriptorFile(t, validDescriptor(rootCAPath, now), 0o600)

	descriptor, err := readAt(filePath, now)
	if err != nil {
		t.Fatalf("readAt() error = %v", err)
	}
	if descriptor.ProtocolVersion != 1 || descriptor.Transport != TransportStreamableHTTP {
		t.Fatalf("unexpected protocol: %+v", descriptor)
	}
	if descriptor.Client.PID != os.Getpid() {
		t.Fatalf("client PID = %d, want %d", descriptor.Client.PID, os.Getpid())
	}
}

func TestReadRejectsInsecureDescriptorPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose POSIX file permission semantics")
	}
	now := time.Now().UTC()
	rootCAPath := filepath.Join(t.TempDir(), "root-ca.pem")
	if err := os.WriteFile(rootCAPath, []byte("test root CA"), 0o600); err != nil {
		t.Fatal(err)
	}
	filePath := writeDescriptorFile(t, validDescriptor(rootCAPath, now), 0o644)

	_, err := readAt(filePath, now)
	if err == nil || !strings.Contains(err.Error(), "permissions") {
		t.Fatalf("readAt() error = %v, want permissions error", err)
	}
}

func TestReadRejectsSymlinkDescriptor(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation may require elevated Windows privileges")
	}
	now := time.Now().UTC()
	directory := t.TempDir()
	rootCAPath := filepath.Join(directory, "root-ca.pem")
	if err := os.WriteFile(rootCAPath, []byte("test root CA"), 0o600); err != nil {
		t.Fatal(err)
	}
	targetPath := writeDescriptorFileIn(t, directory, validDescriptor(rootCAPath, now), 0o600)
	linkPath := filepath.Join(directory, "descriptor-link.json")
	if err := os.Symlink(targetPath, linkPath); err != nil {
		t.Fatal(err)
	}

	_, err := readAt(linkPath, now)
	if err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("readAt() error = %v, want symbolic link error", err)
	}
}

func TestReadRejectsUnsupportedProtocolFields(t *testing.T) {
	now := time.Now().UTC()
	rootCAPath := filepath.Join(t.TempDir(), "root-ca.pem")
	if err := os.WriteFile(rootCAPath, []byte("test root CA"), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(map[string]any)
		want   string
	}{
		{name: "protocol", mutate: func(value map[string]any) { value["protocol"] = "other" }, want: "protocol"},
		{name: "protocol version", mutate: func(value map[string]any) { value["protocolVersion"] = 2 }, want: "protocolVersion"},
		{name: "transport", mutate: func(value map[string]any) { value["transport"] = "stdio" }, want: "transport"},
		{name: "unknown field", mutate: func(value map[string]any) { value["refreshToken"] = "secret" }, want: "unknown field"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := validDescriptor(rootCAPath, now)
			test.mutate(value)
			filePath := writeDescriptorFile(t, value, 0o600)
			_, err := readAt(filePath, now)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("readAt() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestReadRejectsNonLocalEndpoint(t *testing.T) {
	now := time.Now().UTC()
	rootCAPath := filepath.Join(t.TempDir(), "root-ca.pem")
	if err := os.WriteFile(rootCAPath, []byte("test root CA"), 0o600); err != nil {
		t.Fatal(err)
	}
	value := validDescriptor(rootCAPath, now)
	value["endpoint"] = "https://api.example.com/agent-runtime/v1/mcp/client"
	filePath := writeDescriptorFile(t, value, 0o600)

	_, err := readAt(filePath, now)
	if err == nil || !strings.Contains(err.Error(), "loopback HTTPS") {
		t.Fatalf("readAt() error = %v, want endpoint error", err)
	}
}

func TestReadRejectsExpiredDescriptor(t *testing.T) {
	now := time.Now().UTC()
	rootCAPath := filepath.Join(t.TempDir(), "root-ca.pem")
	if err := os.WriteFile(rootCAPath, []byte("test root CA"), 0o600); err != nil {
		t.Fatal(err)
	}
	value := validDescriptor(rootCAPath, now)
	value["expiresAt"] = now.Add(-time.Second).Format(time.RFC3339Nano)
	filePath := writeDescriptorFile(t, value, 0o600)

	_, err := readAt(filePath, now)
	if err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("readAt() error = %v, want expired error", err)
	}
}

func TestReadRejectsInvalidClientProcess(t *testing.T) {
	now := time.Now().UTC()
	rootCAPath := filepath.Join(t.TempDir(), "root-ca.pem")
	if err := os.WriteFile(rootCAPath, []byte("test root CA"), 0o600); err != nil {
		t.Fatal(err)
	}
	value := validDescriptor(rootCAPath, now)
	value["client"] = map[string]any{
		"pid":       0,
		"startedAt": now.Add(time.Minute).Format(time.RFC3339Nano),
	}
	filePath := writeDescriptorFile(t, value, 0o600)

	_, err := readAt(filePath, now)
	if err == nil || !strings.Contains(err.Error(), "client process") {
		t.Fatalf("readAt() error = %v, want client process error", err)
	}
}

func validDescriptor(rootCAPath string, now time.Time) map[string]any {
	return map[string]any{
		"protocol":         Protocol,
		"protocolVersion":  1,
		"transport":        TransportStreamableHTTP,
		"endpoint":         "https://127.0.0.1:61002/agent-runtime/v1/mcp/client",
		"localAccessToken": "process-local-token",
		"rootCaPath":       rootCAPath,
		"expiresAt":        now.Add(5 * time.Minute).Format(time.RFC3339Nano),
		"client": map[string]any{
			"pid":       os.Getpid(),
			"startedAt": now.Add(-time.Minute).Format(time.RFC3339Nano),
		},
	}
}

func writeDescriptorFile(t *testing.T, value map[string]any, mode os.FileMode) string {
	t.Helper()
	return writeDescriptorFileIn(t, t.TempDir(), value, mode)
}

func writeDescriptorFileIn(t *testing.T, directory string, value map[string]any, mode os.FileMode) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(directory, "descriptor.json")
	if err := os.WriteFile(filePath, data, mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filePath, mode); err != nil {
		t.Fatal(err)
	}
	return filePath
}
