package validator

import (
	"testing"

	"github.com/jakejimenez/nlci/internal/definition"
)

func TestValidateFlagDrivenRejectsUnknownFlag(t *testing.T) {
	def := &definition.CLIDefinition{
		Binary: "curl",
		Mode:   "flag_driven",
		RootFlags: []definition.Flag{
			{Name: "json"},
			{Name: "output", Short: "o"},
		},
	}
	result := Validate("curl --not-a-real-flag https://example.com", def)
	if result.Valid {
		t.Fatal("expected validation failure for unknown flag")
	}
	if result.Error == "" {
		t.Fatal("expected validation error message")
	}
}

func TestValidateFlagDrivenAcceptsKnownFlags(t *testing.T) {
	def := &definition.CLIDefinition{
		Binary: "curl",
		Mode:   "flag_driven",
		RootFlags: []definition.Flag{
			{Name: "json"},
			{Name: "output", Short: "o", ValueHint: "file"},
			{Name: "silent", Short: "s"},
			{Name: "verbose", Short: "v"},
		},
	}
	result := Validate("curl --json '{\"ok\":true}' -o out.json https://example.com", def)
	if !result.Valid {
		t.Fatalf("expected validation success, got %+v", result)
	}
	clustered := Validate("curl -sv -oout.json https://example.com", def)
	if !clustered.Valid {
		t.Fatalf("expected clustered short flags to validate, got %+v", clustered)
	}
}
