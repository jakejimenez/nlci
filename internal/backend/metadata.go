package backend

import "context"

// MetadataRequest is the input to a structured-output metadata-generation
// call. Mirrors Request but uses Prompt instead of Intent for clarity — this
// is not a single-command intent, it's a request for structured tool
// metadata derived from help text.
type MetadataRequest struct {
	System string
	Prompt string
}

// MetadataResult mirrors cmd/nlci's EnrichmentMetadata so backends with
// native structured output (e.g. Apple Intelligence's @Generable schemas)
// can produce it directly without a textual YAML round-trip.
type MetadataResult struct {
	Description  string
	SystemPrompt string
	Safety       Safety
	Synonyms     map[string][]string
}

// Safety duplicates definition.Safety in this package to avoid an import
// cycle (definition imports backend transitively in some test paths).
type Safety struct {
	RequireConfirmation []string
	Forbidden           []string
}

// MetadataGenerator is implemented by backends that support native
// structured-output metadata generation. Backends that don't implement it
// fall through to the textual YAML prompt path in cmd/nlci. This is
// idiomatic Go capability detection (cf. io.WriterTo, fs.ReadDirFS).
type MetadataGenerator interface {
	GenerateMetadata(ctx context.Context, r MetadataRequest) (*MetadataResult, error)
}
