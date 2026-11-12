import Foundation
import Observation
import MetamonServices
import MetamonDSL

/// サイドバーの区分(#969)。「作ったアプリ」と「共有されたアプリ」(シェアのリンクから追加・参加したアプリ)。
/// サーバーの一覧(`GET /v1/apps`)は持ち主を返さないので、この Mac で取り込んだ時に覚えておく。
/// 並びは区分をまたいで 1 本(`MacAppLibrary.createdIds`・iOS の変身一覧と同じ)で、区分はその並びを保ったまま分ける。
enum MacSidebarSections {
    struct Split: Equatable {
        var own: [String]
        var shared: [String]
    }

    /// `ids` の並びのまま分ける
    static func split(_ ids: [String], isShared: (String) -> Bool) -> Split {
        var out = Split(own: [], shared: [])
        for id in ids {
            if isShared(id) { out.shared.append(id) } else { out.own.append(id) }
        }
        return out
    }

    /// 区分の中で並べ替えた後の、全体の並び。区分の行が占めていた位置に新しい順で入れ直す(他の区分の位置は動かさない)
    static func reordered(_ all: [String], section: [String], newSection: [String]) -> [String] {
        let members = Set(section)
        var next = newSection.makeIterator()
        return all.map { id in members.contains(id) ? (next.next() ?? id) : id }
    }

    /// ドラッグ(`List` の `onMove`)を区分の中の位置で受けて全体の並びにする
    static func moved(_ all: [String], section: [String], fromOffsets source: IndexSet, toOffset destination: Int) -> [String] {
        var newSection = section
        newSection.move(fromOffsets: source, toOffset: destination)
        return reordered(all, section: section, newSection: newSection)
    }

    /// 右クリックの「上へ / 下へ」(区分の中の `index` 番目へ)
    static func moved(_ all: [String], section: [String], id: String, to index: Int) -> [String] {
        reordered(all, section: section, newSection: AppListEdit.moved(section, id: id, to: index))
    }
}

/// シェアから取り込んだアプリの印(端末内・UserDefaults)。サインアウト・削除で消す
struct MacAppOrigins: @unchecked Sendable {
    static let key = "metamon.mac.sharedAppIds"
    private let defaults: UserDefaults

    init(defaults: UserDefaults = .standard) { self.defaults = defaults }

    var ids: Set<String> { Set(defaults.stringArray(forKey: Self.key) ?? []) }

    func isShared(_ appId: String) -> Bool { ids.contains(appId) }

    func markShared(_ appId: String) {
        var next = ids
        guard next.insert(appId).inserted else { return }
        defaults.set(next.sorted(), forKey: Self.key)
    }

    func remove(_ appId: String) {
        var next = ids
        guard next.remove(appId) != nil else { return }
        defaults.set(next.sorted(), forKey: Self.key)
    }

    func removeAll() { defaults.removeObject(forKey: Self.key) }
}

/// サイドバーの新着の数(#969・iOS の `NewItemsHost` と同じ数え方)。開いていないアプリについて、
/// 前回開いた時刻より後に他の人が作った・更新した共有レコードを数える。基準の時刻は進めない(開いた時に `NewItemsCheck` が進める)。
/// この Mac で一度も開いていない・中身(entity)をまだ取っていないアプリは数えない(初めて開いた時に基準を作るのと同じ)。
/// 数は全ウインドウで 1 つ。どれかのウインドウで開いているアプリは数えない。数え直しは全ウインドウで 1 本・数分に 1 回まで。
@MainActor @Observable
final class MacSidebarBadges {
    static let shared = MacSidebarBadges()

    /// 同じアカウント・同じ一覧なら、この間隔より短くは数え直さない(前面に戻るたびに全アプリへ問い合わせない)
    nonisolated static let minInterval: TimeInterval = 180

    private(set) var counts: [String: Int] = [:]

    @ObservationIgnored private let now: () -> Date

    init(now: @escaping () -> Date = Date.init) { self.now = now }

    func count(_ appId: String) -> Int { counts[appId] ?? 0 }

    /// 開いた(既読)。数えている途中の結果でも戻さない
    func markRead(_ appId: String) {
        counts[appId] = nil
        opened.insert(appId)
    }

    /// 数え直している間に開いたアプリ(結果で印を戻さない)
    @ObservationIgnored private var opened: Set<String> = []
    /// ウインドウごとに中央で開いているアプリ
    @ObservationIgnored private var openByWindow: [UUID: String] = [:]
    /// 最後に数え直しを始めた時の相手(アカウント・一覧)と時刻
    @ObservationIgnored private var lastRun: (key: String, at: Date)?
    @ObservationIgnored private var isRunning = false

    /// どれかのウインドウで開いているアプリ
    var openAppIds: Set<String> { Set(openByWindow.values) }

    /// ウインドウの中央で開いたアプリ(nil は閉じた)。開いたアプリは既読にする
    func setOpen(_ appId: String?, window: UUID) {
        openByWindow[window] = appId
        if let appId { markRead(appId) }
    }

    /// ウインドウを閉じた
    func closeWindow(_ window: UUID) { openByWindow[window] = nil }

    struct Target: Equatable, Sendable {
        var appId: String
        var entities: [String]
    }

    /// 数える相手。共有の entity だけ(private の記録は他の人が書かない)。共有の entity が無ければ nil
    nonisolated static func target(appId: String, entities: [Entity]) -> Target? {
        let shared = entities.filter(\.isShared).map(\.id)
        return shared.isEmpty ? nil : Target(appId: appId, entities: shared)
    }

    /// 全ウインドウから呼ぶ入口。同じアカウント・同じ一覧で `minInterval` 以内に数え直していれば何もしない。
    /// 走っている間に来た分も捨てる(全ウインドウで 1 本)。サインアウトはすぐ消す
    func refreshIfDue(_ targets: [Target], uid: String?, source: (any SharedRecordStampSource)?,
                      store: LastOpenedStore = LastOpenedStore()) async {
        guard let uid, source != nil else {
            lastRun = nil
            counts = [:]
            return
        }
        let key = "\(uid)|" + targets.map { "\($0.appId):\($0.entities.joined(separator: ","))" }.joined(separator: ";")
        let time = now()
        if isRunning { return }
        if let lastRun, lastRun.key == key, time.timeIntervalSince(lastRun.at) < Self.minInterval { return }
        lastRun = (key, time)
        isRunning = true
        defer { isRunning = false }
        await refresh(targets, openAppId: nil, uid: uid, source: source, store: store)
        if Task.isCancelled { lastRun = nil } // 途中で止まった(ウインドウを閉じた等)ら次の呼び出しで数え直す
    }

    /// 数え直す。どれかのウインドウで開いているアプリと `openAppId` は数えない
    func refresh(_ targets: [Target], openAppId: String?, uid: String?,
                 source: (any SharedRecordStampSource)?, store: LastOpenedStore = LastOpenedStore()) async {
        guard let uid, let source else {
            counts = [:]
            return
        }
        opened = []
        var skip = openAppIds
        if let openAppId { skip.insert(openAppId) }
        var next: [String: Int] = [:]
        for target in targets where !skip.contains(target.appId) {
            guard !Task.isCancelled else { return }
            if let count = await Self.peek(target, uid: uid, source: source, store: store), count > 0 {
                next[target.appId] = count
            }
        }
        guard !Task.isCancelled else { return }
        for id in opened.union(openAppIds) { next[id] = nil }
        if let openAppId { next[openAppId] = nil }
        counts = next
    }

    /// 1 アプリ分。前回開いた時刻が無い・1 entity も取れない時は nil
    nonisolated static func peek(_ target: Target, uid: String, source: any SharedRecordStampSource,
                                 store: LastOpenedStore) async -> Int? {
        guard !target.entities.isEmpty, let since = store.lastOpened(appId: target.appId) else { return nil }
        var stamps: [RecordStamp] = []
        var fetched = false
        for entity in target.entities {
            guard let page = try? await source.sharedStamps(appId: target.appId, entity: entity, notBy: uid) else { continue }
            stamps += page
            fetched = true
        }
        return fetched ? NewItemsCounter.count(stamps, me: uid, since: since) : nil
    }

    /// 行の右に出す文字(100 以上は 99+)
    nonisolated static func badgeText(_ count: Int) -> String { count > 99 ? "99+" : "\(count)" }

    /// VoiceOver: 「アプリ名, 新着 N 件」
    static func accessibilityLabel(name: String, count: Int) -> String {
        count > 0 ? L10n.tr("\(name), 新着 \(count) 件") : name
    }
}
