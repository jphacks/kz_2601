import Foundation

// 「新着 N 件」(#268)。他人の行動でのプッシュ(D-21)ができるまでの最低限の代わり。
// アプリを開いた時、前回開いた時刻より後に他の人が作った・更新した共有レコードを数える。

/// 件数を数えるのに要るレコードの一部。`Record` に author が入る前(#378 / #397)でも、ここで author を読む。
/// author は shared / public の entity だけ backend が付けるので、author が無いレコード(private)は数えない。
public struct RecordStamp: Decodable, Sendable, Equatable {
    public var id: String
    public var author: String?
    public var createdAt: String?
    public var updatedAt: String?

    public init(id: String, author: String?, createdAt: String? = nil, updatedAt: String? = nil) {
        self.id = id
        self.author = author
        self.createdAt = createdAt
        self.updatedAt = updatedAt
    }

    /// 作成・更新のうち新しい方。
    public var lastChanged: Date? {
        [createdAt, updatedAt].compactMap { $0.flatMap(Self.parseDate) }.max()
    }

    private static let isoFractional: ISO8601DateFormatter = {
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return f
    }()
    private static let iso = ISO8601DateFormatter()

    /// backend は RFC3339Nano(小数 9 桁まで)。読めない桁数は 3 桁に切り詰めて読む。
    static func parseDate(_ s: String) -> Date? {
        if let d = isoFractional.date(from: s) ?? iso.date(from: s) { return d }
        guard let dot = s.firstIndex(of: ".") else { return nil }
        let tail = s[s.index(after: dot)...]
        let digits = tail.prefix { $0.isNumber }
        return isoFractional.date(from: s[..<dot] + "." + digits.prefix(3) + tail.dropFirst(digits.count))
    }
}

/// 共有レコードの stamp を取る口。`HTTPAppConfigClient` が実装する(モックは実装しない = 新着を出さない)。
public protocol SharedRecordStampSource: Sendable {
    /// `uid` 以外が作った共有レコード。private の entity は空を返す。
    func sharedStamps(appId: String, entity: String, notBy uid: String) async throws -> [RecordStamp]
}

public enum NewItemsCounter {
    /// `since` より後に、自分以外が作った・更新したレコードの数。author の無いもの(private・未送信)は数えない。
    public static func count(_ stamps: [RecordStamp], me uid: String, since: Date) -> Int {
        stamps.filter { s in
            guard let author = s.author, author != uid, let changed = s.lastChanged else { return false }
            return changed > since
        }.count
    }
}

/// アプリごとの「最後に開いた時刻」(端末内・UserDefaults)。
public struct LastOpenedStore: @unchecked Sendable {
    private let defaults: UserDefaults
    private static let prefix = "metamon.newItems.lastOpened."

    public init(defaults: UserDefaults = .standard) { self.defaults = defaults }

    public func lastOpened(appId: String) -> Date? {
        defaults.object(forKey: Self.prefix + appId) as? Date
    }

    public func mark(appId: String, at date: Date) {
        defaults.set(date, forKey: Self.prefix + appId)
    }
}

/// 開いた時の 1 回分。前回時刻が無い(初めて開いた)時は基準だけ作って nil。
/// 1 entity も取れなければ基準を進めない(次に開いた時に数え直す)。
public struct NewItemsCheck: Sendable {
    public var source: any SharedRecordStampSource
    public var store: LastOpenedStore

    public init(source: any SharedRecordStampSource, store: LastOpenedStore = LastOpenedStore()) {
        self.source = source
        self.store = store
    }

    public func run(appId: String, entities: [String], uid: String, now: Date = .now) async -> Int? {
        guard !entities.isEmpty else { return nil }
        let since = store.lastOpened(appId: appId)
        var stamps: [RecordStamp] = []
        var fetched = false
        for entity in entities {
            do {
                stamps += try await source.sharedStamps(appId: appId, entity: entity, notBy: uid)
                fetched = true
            } catch {
                print("[Metamon] newItems: \(entity) failed: \(error.localizedDescription)")
            }
        }
        guard fetched else { return nil }
        store.mark(appId: appId, at: now)
        guard let since else { return nil }
        return NewItemsCounter.count(stamps, me: uid, since: since)
    }
}
