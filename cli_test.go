package modacli_test

import (
	"os/exec"
	"strings"
	"testing"
)

func TestHelpPublishesModaCLIIdentity(t *testing.T) {
	command := exec.Command("go", "run", "./cmd/moda-cli", "--help")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("moda-cli --help failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "moda-cli") {
		t.Fatalf("help output does not identify moda-cli:\n%s", output)
	}
}
