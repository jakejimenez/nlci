import Foundation

/// BridgeInput is the JSON payload sent from Go to the Swift subprocess via stdin.
struct BridgeInput: Codable {
    let system: String
    let schema: String
    let examples: [[String]]
    let intent: String

    // Custom decoder so `"examples": null` or a missing key never crashes.
    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        system   = try c.decode(String.self, forKey: .system)
        schema   = try c.decode(String.self, forKey: .schema)
        examples = try c.decodeIfPresent([[String]].self, forKey: .examples) ?? []
        intent   = try c.decode(String.self, forKey: .intent)
    }
}
