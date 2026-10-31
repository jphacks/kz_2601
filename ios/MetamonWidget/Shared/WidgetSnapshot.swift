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

    /// systemLarge(#486)の「最近の記録」1 行。表示文字列は本体で整形済み。
    struct Row: Codable, Equatable, Identifiable {
        var id: String
        var title: String
        /// 日付など(右寄せの値の左に小さく出す)。
        var detail: String?
        /// 金額などの数値(右寄せ)。
        var value: String?
    }

    /// systemLarge(#486)の「直近 7 日」棒グラフ。先頭 stat と同じ集計を日ごとに出したもの。
    struct Trend: Codable, Equatable {
        struct Point: Codable, Equatable {
            /// その日の 0 時(端末ローカル)。曜日の表示はウィジェット側の言語で付ける。
            var day: Date
            var value: Double
        }
        /// 何を数えたか(field 名。nil = 件数)。
        var metric: String?
        /// 7 日ぶんの合計(集計済み表示文字列)。
        var total: String
        var points: [Point]
    }

    var appName: String
    var stats: [Stat]
    var updatedAt: Date
    /// 以下は systemLarge 用(#486)。古いスナップショット(キー無し)は nil で読める。
    /// 「最近の記録」の entity 名。
    var recentTitle: String? = nil
    /// 新しい順の記録。nil = 出さない(entity が無い・未取得)/ 空 = まだ記録が無い。
    var recent: [Row]? = nil
    var trend: Trend? = nil

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

    /// 更新時刻以外が同じか(同じなら書き込み・reload を省く)。
    func hasSameContent(as other: WidgetSnapshot?) -> Bool {
        guard var other else { return false }
        other.updatedAt = updatedAt
        return self == other
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
        updatedAt: .now,
        recentTitle: "支出",
        recent: [
            Row(id: "1", title: "コンビニ", detail: "10/31(金)", value: "¥680"),
            Row(id: "2", title: "電車", detail: "10/30(木)", value: "¥420"),
            Row(id: "3", title: "ランチ", detail: "10/30(木)", value: "¥1,100"),
            Row(id: "4", title: "ドラッグストア", detail: "10/28(火)", value: "¥2,340"),
            Row(id: "5", title: "カフェ", detail: "10/27(月)", value: "¥520"),
        ],
        trend: Trend(metric: "金額", total: "¥7,860", points: sampleTrendPoints([1200, 0, 2340, 0, 1520, 2180, 680]))
    )

    /// 今日を最後とする日ごとの点(サンプル・プレビュー用)。
    static func sampleTrendPoints(_ values: [Double], endingAt now: Date = .now, calendar: Calendar = .current) -> [Trend.Point] {
        let today = calendar.startOfDay(for: now)
        return values.enumerated().map { i, v in
            Trend.Point(day: calendar.date(byAdding: .day, value: i - (values.count - 1), to: today) ?? today, value: v)
        }
    }
}
