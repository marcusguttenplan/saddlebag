// swift-tools-version: 6.0

import PackageDescription

let package = Package(
    name: "SaddlebagShared",
    platforms: [
        .macOS(.v15)
    ],
    products: [
        .library(
            name: "SaddlebagShared",
            targets: ["SaddlebagShared"]
        )
    ],
    targets: [
        .target(
            name: "SaddlebagShared",
            path: "Sources"
        ),
        .testTarget(
            name: "SaddlebagSharedTests",
            dependencies: ["SaddlebagShared"],
            path: "Tests"
        )
    ]
)
