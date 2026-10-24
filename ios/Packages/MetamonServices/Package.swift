// swift-tools-version: 5.10
// MetamonServices: 外部 I/O の protocol 群(Protocols/)とその実装(Live/ Mock/ Native/)。
// 依存は MetamonDSL と WhisperKit のみ。SwiftUI は import しない(View は MetamonRenderer / アプリ本体)。
import PackageDescription

let package = Package(
    name: "MetamonServices",
    platforms: [.iOS(.v17), .macOS(.v14)],
    products: [
        .library(name: "MetamonServices", targets: ["MetamonServices"]),
    ],
    dependencies: [
        .package(path: "../MetamonDSL"),
        // WhisperKit は v1.0.0 で argmax-oss-swift に統合(旧 URL argmaxinc/WhisperKit は同リポジトリへリダイレクト)。
        // product `WhisperKit` のみリンク(ArgmaxOSS 全部入りは TTS/Speaker を巻き込むので不要)。バージョンは完全固定。
        .package(url: "https://github.com/argmaxinc/argmax-oss-swift", exact: "1.1.1"),
    ],
    targets: [
        .target(
            name: "MetamonServices",
            dependencies: [
                .product(name: "MetamonDSL", package: "MetamonDSL"),
                .product(name: "WhisperKit", package: "argmax-oss-swift"),
            ],
            swiftSettings: [.enableExperimentalFeature("StrictConcurrency=targeted")]
        ),
        .testTarget(
            name: "MetamonServicesTests",
            dependencies: ["MetamonServices"]
        ),
    ],
    swiftLanguageVersions: [.v5]
)
