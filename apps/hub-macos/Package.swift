// swift-tools-version: 5.9
import PackageDescription

let package = Package(
    name: "DJ4Hub",
    platforms: [.macOS(.v13)],
    products: [.executable(name: "DJ4Hub", targets: ["DJ4Hub"])],
    targets: [
        .target(name: "NativeAudio", linkerSettings: [.linkedFramework("AudioToolbox"), .linkedFramework("CoreFoundation")]),
        .executableTarget(name: "DJ4Hub", dependencies: ["NativeAudio"]),
        .testTarget(name: "DJ4HubTests", dependencies: ["DJ4Hub"])
    ]
)
