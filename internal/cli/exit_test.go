package cli

import (
	"errors"
	"fmt"
	"testing"
)

func TestAsExitCode(t *testing.T) {
	code, ok := AsExitCode(ExitCode(3))
	if !ok || code != 3 {
		t.Fatalf("got %d %v", code, ok)
	}
	code, ok = AsExitCode(fmt.Errorf("wrap: %w", ExitCode(1)))
	if !ok || code != 1 {
		t.Fatalf("wrapped: %d %v", code, ok)
	}
	if _, ok := AsExitCode(errors.New("plain")); ok {
		t.Fatal("plain error should not be exit code")
	}
}
