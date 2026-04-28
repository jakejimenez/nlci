import Foundation

/// BridgeOutput is the JSON payload written by the Swift subprocess to stdout.
///
/// In command mode `command`/`explanation` carry the result; in metadata mode
/// `metadata` carries it instead. `--ping` responses populate `capabilities`
/// so callers can detect whether this binary supports the metadata schema
/// without trying it speculatively.
struct BridgeOutput: Codable {
    let mode: String?
    let command: String?
    let explanation: String?
    let metadata: ToolMetadata?
    let capabilities: [String]?
    let error: String?

    init(command: String, explanation: String) {
        self.mode = "command"
        self.command = command
        self.explanation = explanation
        self.metadata = nil
        self.capabilities = nil
        self.error = nil
    }

    init(metadata: ToolMetadata) {
        self.mode = "metadata"
        self.command = nil
        self.explanation = nil
        self.metadata = metadata
        self.capabilities = nil
        self.error = nil
    }

    init(capabilities: [String]) {
        self.mode = nil
        self.command = "ok"
        self.explanation = "Apple Intelligence is available"
        self.metadata = nil
        self.capabilities = capabilities
        self.error = nil
    }

    init(error: String) {
        self.mode = nil
        self.command = nil
        self.explanation = nil
        self.metadata = nil
        self.capabilities = nil
        self.error = error
    }
}

/// Writes a BridgeOutput as a single JSON line to stdout.
func emit(_ output: BridgeOutput) {
    guard let data = try? JSONEncoder().encode(output),
          let json = String(data: data, encoding: .utf8) else {
        fputs("{\"error\":\"internal: failed to encode output\"}\n", stderr)
        return
    }
    print(json)
    fflush(stdout)
}
