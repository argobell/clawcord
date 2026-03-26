package main

import "testing"

func TestNewClawcordCommandIncludesOnboard(t *testing.T) {
	cmd := NewClawcordCommand()

	if cmd.Use != "clawcord" {
		t.Fatalf("Use = %q, want %q", cmd.Use, "clawcord")
	}

	found := false
	for _, sub := range cmd.Commands() {
		if sub.Use == "onboard" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("onboard command not found")
	}
}
