import Foundation
import FoundationModels

/// ToolMetadata is the structured output of `nlci init` enrichment mode.
/// It mirrors the Go EnrichmentMetadata struct in cmd/nlci; the bridge maps
/// 1:1 onto BridgeOutput.metadata. Each field carries a @Guide describing the
/// content the model should produce, anchored in the verified command surface
/// the user prompt provides.
@Generable
struct ToolMetadata: Codable {
    @Guide(description: "A single-line summary of what the tool is for, capped at roughly 80 characters. Capture the purpose, not the surface. Bad: 'docker CLI'. Good: 'Manage Docker containers, images, networks, and volumes'.")
    var description: String

    @Guide(description: "A multi-line system prompt (4-7 lines) suitable to be used directly as the system prompt for an LLM that translates intent into commands for this tool. Mention the binary name, any tool-specific behavior derived from the help text (defaults, required flags), and safety reminders. Avoid generic boilerplate that would apply to any CLI.")
    var systemPrompt: String

    @Guide(description: "Safety patterns: which command substrings need confirmation before execution, and which are absolutely forbidden. Ground each pick in real verbs from the help text or verified command list.")
    var safety: SafetyRules

    @Guide(description: "Map of natural-language synonym words to the command paths or capability names they refer to. Aim for 5-15 entries that cover the common ways a user might phrase intent.")
    var synonyms: [SynonymEntry]
}

/// SafetyRules carry destructive-pattern lists. Both arrays may be empty for
/// non-destructive tools (e.g. read-only query CLIs).
@Generable
struct SafetyRules: Codable {
    @Guide(description: "Command-substring patterns that should prompt the user before running. Use full subcommand paths (e.g. 'docker rm', not just 'rm'). Look for delete/remove/destroy/drop/purge/reset/force/prune verbs in the verified commands. Empty array if the tool has no destructive operations.")
    var requireConfirmation: [String]

    @Guide(description: "Command-substring patterns the user should never run. Reserved for irreversible plus sweeping operations (e.g. 'docker system prune --all --volumes --force'). Usually empty.")
    var forbidden: [String]
}

/// SynonymEntry pairs a natural-language word with one or more command paths
/// or capability names. We use an array of pairs rather than a Swift
/// Dictionary because @Generable's macro support for [String: [String]] is
/// not documented; arrays of structs are the well-trodden path.
@Generable
struct SynonymEntry: Codable {
    @Guide(description: "A short natural-language word a user might say (e.g. 'list', 'delete', 'show', 'clean', 'fetch').")
    var keyword: String

    @Guide(description: "Command paths or capability names from this tool that the keyword refers to. At least one entry. Prefer paths that exist in the verified command list.")
    var targets: [String]
}
