import Foundation

/// BridgeInput is the JSON payload sent from Go to the Swift subprocess via stdin.
/// `mode` selects which @Generable schema the model should produce. Defaults
/// to "command" so older Go binaries that don't set the field continue to get
/// today's behavior.
struct BridgeInput: Codable {
    let mode: String
    let system: String
    let schema: String
    let examples: [[String]]
    let intent: String

    // Custom decoder so missing keys or null never crash.
    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        mode     = try c.decodeIfPresent(String.self, forKey: .mode) ?? "command"
        system   = try c.decode(String.self, forKey: .system)
        schema   = try c.decodeIfPresent(String.self, forKey: .schema) ?? ""
        examples = try c.decodeIfPresent([[String]].self, forKey: .examples) ?? []
        intent   = try c.decode(String.self, forKey: .intent)
    }
}
