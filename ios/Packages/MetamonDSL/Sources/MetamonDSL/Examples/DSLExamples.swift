import Foundation

/// 正典サンプル `docs/dsl/examples/*.json`(backend CI でも検証される)。
/// アプリは xcodegen でこのディレクトリをそのままリソースにバンドルする(iOS 側にコピーを持たない=ドリフト防止)。
public enum DSLExamples {
    public static let names: [String] = {
        let all = [
            "initial",
            "kakeibo",
            "training",
            "walk",
            "ranking",
            "gallery-input",
            "gallery-display",
            "gallery-structure",
            "gallery-web",
            "warikan",
            "bmi",
            "shopping",
            "navigation",
            "post_comments",
            "diary_ai",
            "ai_ideas",
            "medicine",
            "attendance_nearby",
            "water",
            "walk_route",
            "health",
            "chat",
            "shops",
            "timetable",
            "dinner",
            "board",
            "recipes",
            "drive",
            "dashcam",
            "sensors",
            "ramen_timer",
            "receipt_ai",
            "form_check",
            "drive_music",
        ]
        #if APP_STORE_REVIEW
        return all.filter { !reviewExcluded.contains($0) }
        #else
        return all
        #endif
    }()

    /// 審査用ビルド(APP_STORE_REVIEW・#245)で外した部品(nearbyShare / locationTracker / healthStat / dashcam / sensorValue / webView)を使う例。
    public static let reviewExcluded: Set<String> = ["attendance_nearby", "walk_route", "health", "dashcam", "sensors", "gallery-web"]

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
