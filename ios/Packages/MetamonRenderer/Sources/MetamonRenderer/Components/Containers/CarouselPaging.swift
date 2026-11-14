import Foundation

/// carousel のページ送りの計算(Mac の左右ボタン・矢印キー・VoiceOver の増減で使う。#940 / #937)。
/// iOS は指で横にめくるだけなので使わない。描画に依存しないのでテストで確かめる。
enum CarouselPaging {
    /// `page` から `delta` 進めたページ。端で止める(回り込まない)。
    static func step(_ page: Int, by delta: Int, count: Int) -> Int {
        guard count > 0 else { return 0 }
        return min(max(page + delta, 0), count - 1)
    }

    static func canGoBack(_ page: Int, count: Int) -> Bool { count > 1 && page > 0 }
    static func canGoForward(_ page: Int, count: Int) -> Bool { count > 1 && page < count - 1 }

    /// 送りの操作(ボタン・点)を出すか。1 枚以下なら出さない。
    static func showsControls(count: Int) -> Bool { count > 1 }

    /// VoiceOver で読む位置(「2 / 5」)。
    static func positionText(_ page: Int, count: Int) -> String {
        guard count > 0 else { return "" }
        return "\(step(page, by: 0, count: count) + 1) / \(count)"
    }
}
