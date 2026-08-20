package main

import (
	"testing"
)

func TestVersionVariables(t *testing.T) {
	if Version == "" {
		t.Errorf("expected non-empty Version")
	}
	if BuildDate == "" {
		t.Errorf("expected non-empty BuildDate")
	}
}
