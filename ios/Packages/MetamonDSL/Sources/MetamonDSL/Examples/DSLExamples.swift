import Foundation

/// 正典サンプル `docs/dsl/examples/*.json`(backend CI でも検証される)。
/// アプリは xcodegen でこのディレクトリをそのままリソースにバンドルする(iOS 側にコピーを持たない=ドリフト防止)。
public enum DSLExamples {
    public static let names = [
        "initial",
        "kakeibo",
        "training",
        "walk",
        "gallery-input",
        "gallery-display",
        "gallery-structure",
        "gallery-web",
        "warikan",
        "bmi",
        "shopping",
        "navigation",
        "diary_ai",
        "medicine",
        "attendance_nearby",
        "water",
        "walk_route",
        "health",
        "shops",
        "timetable",
        "board",
    ]

    /// 初期状態(音声画面のみ)の DSL 名。
    /// 開発用: 環境変数 `METAMON_INITIAL_EXAMPLE`(simctl は `SIMCTL_CHILD_` 接頭辞)で同梱例を直接起動できる(ギャラリー確認用)。
    public static var initial: String {
        if let name = ProcessInfo.processInfo.environment["METAMON_INITIAL_EXAMPLE"], names.contains(name) { return name }
        return "initial"
    }

    public static func load(_ name: String, bundle: Bundle = .main) throws -> AppDSL {
        try AppDSL.bundled(name, bundle: bundle)
    }

    public static func document(_ name: String, version: Int = 1, bundle: Bundle = .main) throws -> AppDocument {
        try AppDocument.bundled(name, version: version, bundle: bundle)
    }
}
