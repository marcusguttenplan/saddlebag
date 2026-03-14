// swift-tools-version: 6.0

import PackageDescription

let package = Package(
    name: "Saddlebag",
    platforms: [
        .macOS(.v15)
    ],
    targets: [
        .executableTarget(
            name: "Saddlebag",
            path: "Saddlebag",
            exclude: ["Info.plist"],
            linkerSettings: [
                .unsafeFlags([
                    "-Xlinker", "-sectcreate",
                    "-Xlinker", "__TEXT",
                    "-Xlinker", "__info_plist",
                    "-Xlinker", "Saddlebag/Info.plist"
                ])
            ]
        ),
        .testTarget(
            name: "SaddlebagTests",
            dependencies: ["Saddlebag"],
            path: "Tests"
        )
    ]
)
