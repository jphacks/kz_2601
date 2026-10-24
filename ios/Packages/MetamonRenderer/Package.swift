// swift-tools-version: 5.10
// MetamonRenderer: DSL → SwiftUI。コンポーネント1種 = 1ファイル(Components/)+ ComponentRegistry(type 文字列 → View)。
// 依存は MetamonDSL のみ。Services は知らない(ランタイムの能力は `DSLRuntime` protocol で受け取る)。
import PackageDescription

let package = Package(
    name: "MetamonRenderer",
    // 固定文言は Resources/Localizable.xcstrings(日本語がソース・英訳を追加 #44)。`Bundle.module` で引く
    defaultLocalization: "ja",
    platforms: [.iOS(.v17)],
    products: [
        .library(name: "MetamonRenderer", targets: ["MetamonRenderer"]),
    ],
    dependencies: [
        .package(path: "../MetamonDSL"),
    ],
    targets: [
        .target(
            name: "MetamonRenderer",
            dependencies: [
                .product(name: "MetamonDSL", package: "MetamonDSL"),
            ],
            resources: [.process("Resources")],
            swiftSettings: [.enableExperimentalFeature("StrictConcurrency=targeted")]
        ),
        .testTarget(
            name: "MetamonRendererTests",
            dependencies: ["MetamonRenderer"]
        ),
    ],
    swiftLanguageVersions: [.v5]
)
