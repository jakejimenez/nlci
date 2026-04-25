// swift-tools-version:6.0
import PackageDescription

let package = Package(
    name: "NLCIApple",
    platforms: [
        .macOS(.v26) // Foundation Models framework requires macOS 26 (Tahoe)
    ],
    targets: [
        .executableTarget(
            name: "nlci-apple",
            path: "Sources/NLCIApple"
        )
    ]
)
