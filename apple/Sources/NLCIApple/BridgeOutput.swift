import Foundation

/// BridgeOutput is the JSON payload written by the Swift subprocess to stdout.
struct BridgeOutput: Codable {
    let command: String?
    let explanation: String?
    let error: String?

    init(command: String, explanation: String) {
        self.command = command
        self.explanation = explanation
        self.error = nil
    }

    init(error: String) {
        self.command = nil
        self.explanation = nil
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
