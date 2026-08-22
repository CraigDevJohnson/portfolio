package buildinfo

import "testing"

func TestRevisionFallsBackToDevelopmentWhenLinkerValueIsBlank(t *testing.T) {
	previous := revision
	t.Cleanup(func() { revision = previous })
	revision = " \t\n "

	if got := Revision(); got != "development" {
		t.Fatalf("Revision() = %q, want %q", got, "development")
	}
}

func TestRevisionPreservesTheFullLinkerInjectedValue(t *testing.T) {
	previous := revision
	t.Cleanup(func() { revision = previous })
	revision = "0123456789abcdef0123456789abcdef01234567"

	if got := Revision(); got != "0123456789abcdef0123456789abcdef01234567" {
		t.Fatalf("Revision() = %q, want full injected revision", got)
	}
}
