import Foundation

/// BridgeInput is the JSON payload sent from Go to the Swift subprocess via stdin.
struct BridgeInput: Codable {
    let system: String
    let schema: String
    let examples: [[String]]
    let intent: String
}
