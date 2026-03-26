package onboard

import "testing"

func TestNewOnboardCommand(t *testing.T) {
	cmd := NewOnboardCommand()

	if cmd.Use != "onboard" {
		t.Fatalf("Use = %q, want %q", cmd.Use, "onboard")
	}
	if len(cmd.Aliases) != 1 || cmd.Aliases[0] != "o" {
		t.Fatalf("Aliases = %#v, want []string{\"o\"}", cmd.Aliases)
	}
	if cmd.Run == nil {
		t.Fatal("Run is nil")
	}
}
