// swift-tools-version: 5.10
// MetamonDSL: DSL v1(docs/dsl/dsl.schema.json)の Swift 表現・グリッド計算・レコード集計。
// 依存ゼロ(Foundation のみ)。UIKit/SwiftUI を import しないので macOS の `swift test` で回る。
import PackageDescription

let package = Package(
    name: "MetamonDSL",
    platforms: [.iOS(.v17), .macOS(.v14)],
    products: [
        .library(name: "MetamonDSL", targets: ["MetamonDSL"]),
    ],
    targets: [
        .target(
            name: "MetamonDSL",
            dependencies: [],
            swiftSettings: [.enableExperimentalFeature("StrictConcurrency=targeted")]
        ),
        .testTarget(
            name: "MetamonDSLTests",
            dependencies: ["MetamonDSL"]
        ),
    ],
    swiftLanguageVersions: [.v5]
)
