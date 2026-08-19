// swift-tools-version: 6.2

import PackageDescription

let package = Package(
    name: "KeySession",
    platforms: [.macOS(.v14)],
    products: [
        .executable(name: "KeySessionApp", targets: ["KeySessionApp"]),
        .executable(name: "KeySessionContractCheck", targets: ["KeySessionContractCheck"]),
        .library(name: "KeySessionKit", targets: ["KeySessionKit"]),
    ],
    dependencies: [
        .package(url: "https://github.com/sparkle-project/Sparkle", exact: "2.9.5"),
    ],
    targets: [
        .target(name: "KeySessionKit"),
        .executableTarget(
            name: "KeySessionApp",
            dependencies: [
                "KeySessionKit",
                .product(name: "Sparkle", package: "Sparkle"),
            ]
        ),
        .executableTarget(name: "KeySessionContractCheck", dependencies: ["KeySessionKit"]),
        .testTarget(name: "KeySessionKitTests", dependencies: ["KeySessionKit"]),
    ]
)
