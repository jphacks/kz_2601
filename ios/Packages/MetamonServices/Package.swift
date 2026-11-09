// swift-tools-version: 5.10
// MetamonServices: 外部 I/O の protocol 群(Protocols/)とその実装(Live/ Mock/ Native/)。
// 依存は MetamonDSL・WhisperKit・FirebaseAuth・FirebaseCrashlytics・FirebaseAppCheck のみ。SwiftUI は import しない(View は MetamonRenderer / アプリ本体)。
import PackageDescription

let package = Package(
    name: "MetamonServices",
    // 固定文言は Resources/Localizable.xcstrings(日本語がソース・英訳を併記)。`Bundle.module` で引く
    defaultLocalization: "ja",
    platforms: [.iOS(.v17), .macOS(.v14)],
    products: [
        .library(name: "MetamonServices", targets: ["MetamonServices"]),
    ],
    dependencies: [
        .package(path: "../MetamonDSL"),
        // WhisperKit は v1.0.0 で argmax-oss-swift に統合(旧 URL argmaxinc/WhisperKit は同リポジトリへリダイレクト)。
        // product `WhisperKit` のみリンク(ArgmaxOSS 全部入りは TTS/Speaker を巻き込むので不要)。バージョンは完全固定。
        .package(url: "https://github.com/argmaxinc/argmax-oss-swift", exact: "1.1.1"),
        // Firebase は Auth とクラッシュ報告(Crashlytics・#296)と App Check(#318)だけ(Analytics 等は入れない=ビルド時間と権限説明を増やさない)。バージョンは完全固定。
        .package(url: "https://github.com/firebase/firebase-ios-sdk", exact: "13.1.1"),
    ],
    targets: [
        .target(
            name: "MetamonServices",
            dependencies: [
                .product(name: "MetamonDSL", package: "MetamonDSL"),
                .product(name: "WhisperKit", package: "argmax-oss-swift"),
                .product(name: "FirebaseAuth", package: "firebase-ios-sdk"),
                .product(name: "FirebaseCrashlytics", package: "firebase-ios-sdk"),
                // App Check(#318): App Attest / DeviceCheck / デバッグ用のトークンを ai・appconfig に付ける
                .product(name: "FirebaseAppCheck", package: "firebase-ios-sdk"),
            ],
            resources: [.process("Resources")],
            swiftSettings: [.enableExperimentalFeature("StrictConcurrency=targeted")]
        ),
        .testTarget(
            name: "MetamonServicesTests",
            dependencies: ["MetamonServices"]
        ),
    ],
    swiftLanguageVersions: [.v5]
)
