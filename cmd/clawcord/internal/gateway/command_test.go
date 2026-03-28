package gateway

import "testing"

func TestNewGatewayCommand(t *testing.T) {
	cmd := NewGatewayCommand()

	if cmd.Use != "gateway" {
		t.Fatalf("Use = %q, want gateway", cmd.Use)
	}
	if len(cmd.Aliases) != 1 || cmd.Aliases[0] != "g" {
		t.Fatalf("Aliases = %#v, want [g]", cmd.Aliases)
	}
	if cmd.Flags().Lookup("debug") == nil {
		t.Fatal("debug flag not found")
	}
}
