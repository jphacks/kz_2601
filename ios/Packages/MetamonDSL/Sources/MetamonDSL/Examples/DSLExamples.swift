import Foundation

/// 正典サンプル `docs/dsl/examples/*.json`(backend CI でも検証される 4 本)。
/// アプリは xcodegen でこのディレクトリをそのままリソースにバンドルする(iOS 側にコピーを持たない=ドリフト防止)。
public enum DSLExamples {
    public static let names = ["initial", "kakeibo", "training", "walk", "gallery-display"]

    /// 初期状態(音声画面のみ)の DSL 名。
    public static let initial = "initial"

    public static func load(_ name: String, bundle: Bundle = .main) throws -> AppDSL {
        try AppDSL.bundled(name, bundle: bundle)
    }

    public static func document(_ name: String, version: Int = 1, bundle: Bundle = .main) throws -> AppDocument {
        try AppDocument.bundled(name, version: version, bundle: bundle)
    }
}
