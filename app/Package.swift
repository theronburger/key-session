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
    targets: [
        .target(name: "KeySessionKit"),
        .executableTarget(name: "KeySessionApp", dependencies: ["KeySessionKit"]),
        .executableTarget(name: "KeySessionContractCheck", dependencies: ["KeySessionKit"]),
        .testTarget(name: "KeySessionKitTests", dependencies: ["KeySessionKit"]),
    ]
)
