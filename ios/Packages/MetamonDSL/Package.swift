// swift-tools-version: 5.10
// MetamonDSL: DSL v1(docs/dsl/dsl.schema.json)の Swift 表現・グリッド計算・レコード集計。
// 依存ゼロ(Foundation のみ)。UIKit/SwiftUI を import しないので macOS の `swift test` で回る。
import PackageDescription

let package = Package(
    name: "MetamonDSL",
    // 固定文言は Resources/Localizable.xcstrings(日本語がソース・英訳を併記)。`Bundle.module` で引く
    defaultLocalization: "ja",
    platforms: [.iOS(.v17), .macOS(.v14)],
    products: [
        .library(name: "MetamonDSL", targets: ["MetamonDSL"]),
    ],
    targets: [
        .target(
            name: "MetamonDSL",
            dependencies: [],
            resources: [.process("Resources")],
            swiftSettings: [.enableExperimentalFeature("StrictConcurrency=targeted")]
        ),
        .testTarget(
            name: "MetamonDSLTests",
            dependencies: ["MetamonDSL"]
        ),
    ],
    swiftLanguageVersions: [.v5]
)
