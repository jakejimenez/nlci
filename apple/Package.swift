// swift-tools-version:6.0
import PackageDescription

let package = Package(
    name: "NLCIApple",
    platforms: [
        .macOS("26.0") // Foundation Models requires macOS 26 (Tahoe); use string form until Xcode ships the .v26 enum
    ],
    targets: [
        .executableTarget(
            name: "nlci-apple",
            path: "Sources/NLCIApple"
        )
    ]
)
