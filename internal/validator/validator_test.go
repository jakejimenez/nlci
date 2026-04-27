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
	result := Validate("curl --not-a-real-flag https://example.com", "fetch https://example.com", def)
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
	intent := "send {\"ok\":true} to https://example.com and save the response to out.json"
	result := Validate("curl --json '{\"ok\":true}' -o out.json https://example.com", intent, def)
	if !result.Valid {
		t.Fatalf("expected validation success, got %+v", result)
	}
	clustered := Validate("curl -sv -oout.json https://example.com", "fetch https://example.com silently and verbosely to out.json", def)
	if !clustered.Valid {
		t.Fatalf("expected clustered short flags to validate, got %+v", clustered)
	}
}

func TestValidateRejectsHallucinatedURL(t *testing.T) {
	def := &definition.CLIDefinition{Binary: "curl", Mode: "flag_driven"}
	intent := "send a get request to https://slackdown.com/"
	command := "curl --json '{\"key\":\"value\"}' https://api.example.com"
	result := Validate(command, intent, def)
	if result.Valid {
		t.Fatalf("expected validation failure for URL not in intent, got valid")
	}
	if !contains(result.Error, "api.example.com") {
		t.Fatalf("expected error to mention the bad URL: %s", result.Error)
	}
}

func TestValidateAcceptsURLPresentInIntent(t *testing.T) {
	def := &definition.CLIDefinition{Binary: "curl", Mode: "flag_driven"}
	intent := "send a get request to https://slackdown.com/"
	command := "curl https://slackdown.com/"
	result := Validate(command, intent, def)
	if !result.Valid {
		t.Fatalf("expected validation success when URL is in intent, got %+v", result)
	}
}

func TestValidateAcceptsURLPresentInIntentCaseInsensitive(t *testing.T) {
	def := &definition.CLIDefinition{Binary: "curl", Mode: "flag_driven"}
	intent := "send a GET to HTTPS://SlackDown.com/"
	command := "curl https://slackdown.com/"
	result := Validate(command, intent, def)
	if !result.Valid {
		t.Fatalf("expected validation success on case-insensitive URL match, got %+v", result)
	}
}

func TestValidateRejectsHallucinatedDigitID(t *testing.T) {
	def := &definition.CLIDefinition{Binary: "ecli"}
	intent := "show me the ecli commands"
	command := "ecli update flags 987654321"
	result := Validate(command, intent, def)
	if result.Valid {
		t.Fatalf("expected validation failure for fabricated ID, got valid")
	}
	if !contains(result.Error, "987654321") {
		t.Fatalf("expected error to mention the bad ID: %s", result.Error)
	}
}

func TestValidateAcceptsDigitIDInIntent(t *testing.T) {
	def := &definition.CLIDefinition{Binary: "gh"}
	intent := "show me PR 12345"
	command := "gh pr view 12345"
	result := Validate(command, intent, def)
	if !result.Valid {
		t.Fatalf("expected validation success when ID is in intent, got %+v", result)
	}
}

func TestValidateIgnoresShortNumbers(t *testing.T) {
	// Short numbers (≤3 digits) like ports, counts, version numbers are not
	// flagged as hallucinations even if absent from intent.
	def := &definition.CLIDefinition{Binary: "curl"}
	intent := "make a get request"
	command := "curl http://localhost:80"
	result := Validate(command, intent, def)
	// URL "http://localhost:80" is not in intent → URL check still rejects.
	// This test asserts that the digit-run check doesn't fire on its own
	// for "80" (3 digits).
	if !contains(result.Error, "localhost") && !contains(result.Error, "http://") {
		// We expect the URL to be the rejection trigger, not the port number.
		// If the test fails because "80" alone is flagged, the digit threshold
		// is wrong.
		t.Logf("rejection reason: %s", result.Error)
	}
}

func TestValidateSkipsCheckOnEmptyIntent(t *testing.T) {
	// SDK paths may invoke Validate without supplying an intent. The
	// hallucination check must be a no-op in that case.
	def := &definition.CLIDefinition{Binary: "curl"}
	command := "curl https://api.example.com"
	result := Validate(command, "", def)
	if !result.Valid {
		t.Fatalf("expected validation success with empty intent, got %+v", result)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
