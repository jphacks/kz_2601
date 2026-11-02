// 審査用ビルド(APP_STORE_REVIEW)は共有を外しているので、仲間探し(#507)も外す。
#if !APP_STORE_REVIEW
import Foundation
import Observation
import MetamonDSL

/// 1 つのアプリの「届いた / 送った / つながった」(#507)。操作は楽観的に先に反映し、失敗したら操作前に戻して throw する。
/// サーバーが正: 成功した応答と `load` の結果で置き換える。
@MainActor
@Observable
public final class MatchingStore {
    public let appId: String
    private let client: any MatchingClient

    /// box → 一覧。キーが無い = まだ読んでいない
    public private(set) var boxes: [MatchBox: [MatchInterest]] = [:]
    /// 読めなかった box(行の代わりに「読み込めませんでした」)
    public private(set) var failed: Set<MatchBox> = []
    /// 送信中の相手(ボタンを押せなくする)
    public private(set) var busy: Set<String> = []

    public init(appId: String, client: any MatchingClient) {
        self.appId = appId
        self.client = client
    }

    public func items(_ box: MatchBox) -> [MatchInterest]? { boxes[box] }

    /// 相手 uid とのつながり(プロフィールの行のボタン)。読んでいない box は無いものとして扱う。
    public func state(of uid: String) -> InterestState {
        if boxes[.matches]?.contains(where: { $0.uid == uid }) == true { return .matched }
        if boxes[.outgoing]?.contains(where: { $0.uid == uid }) == true { return .sent }
        if boxes[.incoming]?.contains(where: { $0.uid == uid }) == true { return .incoming }
        return .none
    }

    public func isBusy(_ uid: String) -> Bool { busy.contains(uid) }

    // MARK: 読む

    public func load(_ box: MatchBox) async throws {
        do {
            let items = try await client.list(appId: appId, box: box)
            guard !Task.isCancelled else { return }
            boxes[box] = items
            failed.remove(box)
        } catch {
            if boxes[box] == nil { failed.insert(box) }
            throw error
        }
    }

    // MARK: 操作(楽観的)

    /// 相手のプロフィール(`recordId`・作者 `uid`)に送る。相手から届いていれば、その場でつながる。
    @discardableResult
    public func send(to uid: String, recordId: String, fromRecordId: String) async throws -> MatchInterest {
        try await optimistic(uid) {
            if let i = self.boxes[.incoming]?.firstIndex(where: { $0.uid == uid }) {
                var hit = self.boxes[.incoming]!.remove(at: i)
                hit.status = "matched"
                self.insert(hit, into: .matches)
            } else {
                self.insert(MatchInterest(uid: uid, profileRecordId: recordId), into: .outgoing)
            }
        } call: {
            try await self.client.sendInterest(appId: self.appId, recordId: recordId, fromRecordId: fromRecordId, ageConfirmed: true)
        }
    }

    /// 届いたものを承認する → つながる。
    @discardableResult
    public func accept(_ uid: String) async throws -> MatchInterest {
        try await optimistic(uid) {
            guard var hit = self.remove(uid, from: .incoming) else { return }
            hit.status = "matched"
            self.insert(hit, into: .matches)
        } call: {
            try await self.client.accept(appId: self.appId, uid: uid, ageConfirmed: true)
        }
    }

    /// 届いたものを見送る(相手には知らされない)。
    public func decline(_ uid: String) async throws {
        try await mutate(uid) { _ = self.remove(uid, from: .incoming) } call: {
            try await self.client.decline(appId: self.appId, uid: uid)
        }
    }

    /// 送ったものを取り下げる。
    public func withdraw(_ uid: String) async throws {
        try await mutate(uid) { _ = self.remove(uid, from: .outgoing) } call: {
            try await self.client.decline(appId: self.appId, uid: uid)
        }
    }

    /// つながりを解除する。
    public func unmatch(_ uid: String) async throws {
        try await mutate(uid) { _ = self.remove(uid, from: .matches) } call: {
            try await self.client.unmatch(appId: self.appId, uid: uid)
        }
    }

    /// ブロックした人を全部の一覧から消す(サーバーも返さなくなる)。
    public func forget(_ uid: String) {
        for box in MatchBox.allCases { _ = remove(uid, from: box) }
    }

    // MARK: 内部

    private func mutate(_ uid: String, apply: () -> Void, call: () async throws -> Void) async throws {
        let before = boxes
        busy.insert(uid)
        defer { busy.remove(uid) }
        apply()
        do {
            try await call()
        } catch {
            boxes = before
            throw error
        }
    }

    private func optimistic(_ uid: String, apply: () -> Void, call: () async throws -> MatchInterest) async throws -> MatchInterest {
        var out: MatchInterest?
        try await mutate(uid, apply: apply) {
            let result = try await call()
            out = result
            self.settle(result)
        }
        return out ?? MatchInterest(uid: uid)
    }

    /// サーバーの応答で、その人の行を正しい box に置き直す。
    private func settle(_ result: MatchInterest) {
        for box in MatchBox.allCases { _ = remove(result.uid, from: box) }
        insert(result, into: result.isMatched ? .matches : .outgoing)
    }

    /// 新しいものを先頭に。読んでいない box にも足す(行のボタンがすぐ変わる。次の `load` でサーバーの値に置き換わる)。
    private func insert(_ item: MatchInterest, into box: MatchBox) {
        var list = boxes[box] ?? []
        list.removeAll { $0.uid == item.uid }
        list.insert(item, at: 0)
        boxes[box] = list
    }

    private func remove(_ uid: String, from box: MatchBox) -> MatchInterest? {
        guard var list = boxes[box], let i = list.firstIndex(where: { $0.uid == uid }) else { return nil }
        let hit = list.remove(at: i)
        boxes[box] = list
        return hit
    }
}

/// 「18 歳以上です」の確認(#507)。初回だけ聞き、端末に覚える。年齢・生年月日は保存しない。
public struct MatchingAgeGate: Sendable {
    public static let key = "matching.ageConfirmed.v1"
    private let defaults: UserDefaults

    public init(defaults: UserDefaults = .standard) {
        self.defaults = defaults
    }

    public var isConfirmed: Bool { defaults.bool(forKey: Self.key) }

    public func confirm() { defaults.set(true, forKey: Self.key) }

    /// サーバーが確認を求めた(400 age_unconfirmed)時に、次は聞き直す。
    public func reset() { defaults.removeObject(forKey: Self.key) }
}
#endif
