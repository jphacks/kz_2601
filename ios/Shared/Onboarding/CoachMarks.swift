import Foundation
import Observation

/// 見た目から分からない操作を、使える場面に初めて来た時に 1 つずつ教える吹き出し(#1441)。
/// 一覧は docs/ios/hidden-gestures.md。並び順 = 出す順番(大事なものから)。
/// iOS と Mac で共有する(Mac は後で同じ順番・同じ保存先で出す)。文言は各アプリ側(iOS は `CoachMarkCopy`)。
enum CoachMark: String, CaseIterable, Comparable {
    /// マイクのボタンの長押しで変身一覧へ戻る
    case fabLongPress
    /// マイクのボタンをドラッグで動かす・端に寄せて隠す
    case fabDrag
    /// 変身一覧のカードを長押しで並べ替える
    case switcherReorder

    private var order: Int { Self.allCases.firstIndex(of: self) ?? 0 }
    static func < (a: CoachMark, b: CoachMark) -> Bool { a.order < b.order }

    /// マイクのボタン(FAB)に出すもの
    static let fabMarks: Set<CoachMark> = [.fabLongPress, .fabDrag]
}

/// どれを出すかの決まり(保存から切り離した純粋な判定・ユニットテスト用)。
enum CoachMarkSchedule {
    /// 前に出したもの(閉じる前にアプリを閉じた時も、次はそれを出し直す)
    struct LastShown: Equatable {
        var mark: CoachMark
        var at: Date
    }

    /// いま出すもの。
    /// - 「もう出さない」を選んだら出さない
    /// - 閉じた・実際に操作したものは出さない
    /// - 出したまま閉じていないものがこの場面で使えるなら、それを続けて出す(一度に 1 つ)
    /// - 新しいものは 1 日 1 つまで(初日に全部を出さない)。設定の「もう一度見る」の間は続けて出す
    static func next(eligible: Set<CoachMark>, done: Set<CoachMark>, optedOut: Bool, replaying: Bool,
                     lastShown: LastShown?, now: Date, calendar: Calendar = .current) -> CoachMark? {
        guard !optedOut else { return nil }
        let candidates = CoachMark.allCases.filter { eligible.contains($0) && !done.contains($0) }
        if let last = lastShown, candidates.contains(last.mark) { return last.mark }
        if !replaying, let last = lastShown, calendar.isDate(last.at, inSameDayAs: now) { return nil }
        return candidates.first
    }
}

/// 吹き出しを閉じたか・操作したか・「もう出さない」を端末に保存する(ヒントごと)。
@MainActor
@Observable
final class CoachMarkStore {
    nonisolated static let doneKeyPrefix = "metamon.coachMark.done."
    nonisolated static let optOutKey = "metamon.coachMark.optOut"
    nonisolated static let lastShownKey = "metamon.coachMark.lastShown"
    nonisolated static let lastShownAtKey = "metamon.coachMark.lastShownAt"

    static let shared = CoachMarkStore(defaults: .standard)

    private(set) var done: Set<CoachMark> = []
    private(set) var optedOut = false
    private(set) var lastShown: CoachMarkSchedule.LastShown?
    /// 設定の「もう一度見る」の後(保存しない): 1 日 1 つの間隔を空けずに続けて出す
    private(set) var replaying = false
    /// 自動テスト用に出さない(保存しない)
    private(set) var suppressed = false
    @ObservationIgnored private let defaults: UserDefaults
    @ObservationIgnored private let calendar: Calendar

    init(defaults: UserDefaults, calendar: Calendar = .current) {
        self.defaults = defaults
        self.calendar = calendar
        reload()
    }

    /// いま出すもの(`eligible` = この場面で使える操作)。
    func next(eligible: Set<CoachMark>, now: Date = Date()) -> CoachMark? {
        guard !suppressed else { return nil }
        return CoachMarkSchedule.next(eligible: eligible, done: done, optedOut: optedOut, replaying: replaying,
                                      lastShown: lastShown, now: now, calendar: calendar)
    }

    func isDone(_ mark: CoachMark) -> Bool { done.contains(mark) }

    /// 画面に出した時。同じものを出し直す時は時刻を変えない(1 日 1 つの数え方を崩さない)。
    func markShown(_ mark: CoachMark, now: Date = Date()) {
        guard lastShown?.mark != mark else { return }
        lastShown = .init(mark: mark, at: now)
        defaults.set(mark.rawValue, forKey: Self.lastShownKey)
        defaults.set(now, forKey: Self.lastShownAtKey)
    }

    /// 「わかった」で閉じた時。次から出さない。
    func dismiss(_ mark: CoachMark) { finish(mark) }

    /// 実際にその操作をした時(吹き出しを出す前でも)。次から出さない。
    func markPerformed(_ mark: CoachMark) { finish(mark) }

    /// 「もう出さない」: 残りもすべて出さない。
    func optOut() {
        optedOut = true
        defaults.set(true, forKey: Self.optOutKey)
    }

    /// 設定の「操作のヒントをもう一度見る」: すべて未読に戻し、間隔を空けずに順に出す。
    func replay() {
        for mark in CoachMark.allCases { defaults.removeObject(forKey: Self.doneKey(mark)) }
        defaults.removeObject(forKey: Self.optOutKey)
        defaults.removeObject(forKey: Self.lastShownKey)
        defaults.removeObject(forKey: Self.lastShownAtKey)
        reload()
        replaying = true
        suppressed = false
    }

    /// 保存先を直接消した後(アカウント削除)に合わせ直す。
    func reload() {
        done = Set(CoachMark.allCases.filter { defaults.bool(forKey: Self.doneKey($0)) })
        optedOut = defaults.bool(forKey: Self.optOutKey)
        if let raw = defaults.string(forKey: Self.lastShownKey), let mark = CoachMark(rawValue: raw),
           let at = defaults.object(forKey: Self.lastShownAtKey) as? Date {
            lastShown = .init(mark: mark, at: at)
        } else {
            lastShown = nil
        }
    }

    nonisolated static func doneKey(_ mark: CoachMark) -> String { doneKeyPrefix + mark.rawValue }

    private func finish(_ mark: CoachMark) {
        guard !done.contains(mark) else { return }
        done.insert(mark)
        defaults.set(true, forKey: Self.doneKey(mark))
        if replaying, done.isSuperset(of: CoachMark.allCases) { replaying = false }
    }

    /// UI テスト・ヘッドレス E2E 用(DEBUG のみ): `METAMON_COACH_MARKS=reset` で未読から、`=off` で出さない。
    /// 未指定で `METAMON_AI_CONSENT=granted`(既存の自動テスト)なら出さない(FAB の上の吹き出しで画面を塞がない)。
    func applyLaunchOverride(environment: [String: String] = ProcessInfo.processInfo.environment) {
        #if DEBUG
        switch environment["METAMON_COACH_MARKS"] {
        case "reset":
            replay()
        case "off":
            suppressed = true
        case nil where environment["METAMON_AI_CONSENT"] == "granted":
            suppressed = true
        default:
            break
        }
        #endif
    }
}
