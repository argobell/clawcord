package main

import "testing"

func TestNewClawcordCommandIncludesCommands(t *testing.T) {
	cmd := NewClawcordCommand()

	if cmd.Use != "clawcord" {
		t.Fatalf("Use = %q, want %q", cmd.Use, "clawcord")
	}

	found := map[string]bool{
		"onboard": false,
		"agent":   false,
		"gateway": false,
	}
	for _, sub := range cmd.Commands() {
		if _, ok := found[sub.Use]; ok {
			found[sub.Use] = true
		}
	}
	for name, ok := range found {
		if !ok {
			t.Fatalf("%s command not found", name)
		}
	}
}
