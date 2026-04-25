package definition

import "testing"

func TestParseFlagsWithValueHints(t *testing.T) {
	help := `Usage: curl [options...] <url>
 -d, --data <data>           HTTP POST data
 -o, --output <file>         Write to file instead of stdout
 -v, --verbose               Make the operation more talkative`

	flags := parseFlagsWithValueHints(help)
	if len(flags) != 3 {
		t.Fatalf("expected 3 flags, got %d", len(flags))
	}
	if flags[0].Name != "data" || flags[0].ValueHint != "data" {
		t.Fatalf("unexpected data flag: %+v", flags[0])
	}
	if flags[1].Name != "output" || flags[1].ValueHint != "file" {
		t.Fatalf("unexpected output flag: %+v", flags[1])
	}
}

func TestSynthesizeCapabilities(t *testing.T) {
	flags := []Flag{
		{Name: "data", Description: "HTTP POST data"},
		{Name: "header", Description: "Pass custom header(s) to server"},
		{Name: "user", Description: "Server user and password"},
		{Name: "output", Description: "Write to file instead of stdout"},
		{Name: "location", Description: "Follow redirects"},
		{Name: "verbose", Description: "Make the operation more talkative"},
	}
	capabilities := synthesizeCapabilities(flags)
	if len(capabilities) < 4 {
		t.Fatalf("expected multiple capabilities, got %+v", capabilities)
	}
	if capabilities[0].Name == "" {
		t.Fatalf("capability name should not be empty: %+v", capabilities)
	}
}
