package main

import (
	"strings"
	"testing"

	"github.com/if-ltd/moda-cli/internal/cli"
)

func TestDirectModeAvailabilityError(t *testing.T) {
	tests := []struct {
		name    string
		command cli.Command
		wantErr bool
	}{
		{name: "client tools remain available", command: cli.Command{Name: "tools", Mode: "client"}},
		{name: "direct tools are unavailable", command: cli.Command{Name: "tools", Mode: "direct"}, wantErr: true},
		{name: "direct authentication is unavailable", command: cli.Command{Name: "auth-status"}, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := directModeAvailabilityError(test.command)
			if (err != nil) != test.wantErr {
				t.Fatalf("directModeAvailabilityError() error = %v, wantErr %v", err, test.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), "not implemented") {
				t.Fatalf("directModeAvailabilityError() error = %q, want an explicit not implemented message", err)
			}
		})
	}
}
