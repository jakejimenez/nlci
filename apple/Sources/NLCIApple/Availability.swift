import Foundation
import FoundationModels

/// Checks whether Apple Intelligence is available on this device.
/// Returns nil if available, or an error string to include in BridgeOutput.
func checkAvailability() -> String? {
    switch SystemLanguageModel.default.availability {
    case .available:
        return nil
    case .unavailable(let reason):
        return "unavailable:\(reason)"
    @unknown default:
        return "unavailable:unknown"
    }
}
