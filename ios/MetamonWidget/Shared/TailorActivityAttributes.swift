import ActivityKit
import Foundation

/// ライブアクティビティ(#423)の受け渡し。本体と MetamonWidget の両ターゲットにコンパイルする(project.yml)。
/// 更新は本体のローカル更新だけ(push しない)。時間は `clock` の日付から OS が描くので、アプリが止まっていても進む。
struct TailorActivityAttributes: ActivityAttributes, Hashable {
    struct ContentState: Codable, Hashable {
        var title: String
        var subtitle: String?
        var clock: Clock?
        /// 添える数字(stat の表示文字列 / progressBar の %)
        var value: String?
        var valueLabel: String?
        /// progressBar 由来の 0...1(超過は 1 で頭打ちして描く)
        var progress: Double?
        /// countdown が 0 になった(終了表示のまま少し残す)
        var finished: Bool
    }

    enum Clock: Codable, Hashable {
        /// 残り時間(start...end を数え下げる)
        case countdown(start: Date, end: Date)
        /// 経過時間(since から数え上げる)
        case elapsed(since: Date)
        /// 一時停止中の表示秒
        case paused(seconds: Int)
    }

    var appId: String
    var appName: String
    /// SF Symbols 名
    var icon: String
    /// 1 つの表示を指すキー: タイマーは "timer:<部品キー>"、startActivity は "app:<appId>"
    var key: String
    /// 記録が変わるたびに数字を取り直す stat / progressBar の部品 id
    var statId: String?

    static func format(seconds: Int) -> String {
        let s = max(seconds, 0)
        let h = s / 3600, m = (s % 3600) / 60, sec = s % 60
        return h > 0 ? String(format: "%d:%02d:%02d", h, m, sec) : String(format: "%02d:%02d", m, sec)
    }
}
