import Foundation

/// 本体 → ウィジェットの受け渡しデータ。App Group の `UserDefaults` に JSON で 1 件だけ置く。
/// 集計は本体(`WidgetSnapshotWriter` / `Aggregator`)で済ませ、ウィジェットは文字列を描画するだけ(Renderer/DSL 非依存)。
/// このファイルは Metamon 本体と MetamonWidget の両ターゲットにコンパイルされる(project.yml)。
struct WidgetSnapshot: Codable, Equatable {
    struct Stat: Codable, Equatable, Identifiable {
        var id: String { key }
        /// entity|aggregate|field|label。DSL 改変後も同じ stat なら前回値を引き継ぐためのキー。
        var key: String
        var label: String
        /// 集計済み表示文字列(例 "¥12,340" / "3 件")。nil = レコード未取得。
        var value: String?
    }

    var appName: String
    var stats: [Stat]
    var updatedAt: Date

    static let appGroupID = "group.jp.kz2601.metamon"
    private static let storageKey = "metamon.widget.snapshot"

    static var defaults: UserDefaults? { UserDefaults(suiteName: appGroupID) }

    static func load() -> WidgetSnapshot? {
        guard let data = defaults?.data(forKey: storageKey) else { return nil }
        return try? JSONDecoder().decode(WidgetSnapshot.self, from: data)
    }

    func save() {
        guard let data = try? JSONEncoder().encode(self) else { return }
        Self.defaults?.set(data, forKey: Self.storageKey)
    }

    static func clear() {
        defaults?.removeObject(forKey: storageKey)
    }

    /// ギャラリー/プレースホルダ用のサンプル(家計簿)。
    static let sample = WidgetSnapshot(
        appName: "家計簿",
        stats: [
            Stat(key: "expense|sum|amount|今月の支出", label: "今月の支出", value: "¥12,340"),
            Stat(key: "expense|count||今月の件数", label: "今月の件数", value: "8 件"),
            Stat(key: "expense|avg|amount|1 件あたり", label: "1 件あたり", value: "¥1,542"),
        ],
        updatedAt: .now
    )
}
