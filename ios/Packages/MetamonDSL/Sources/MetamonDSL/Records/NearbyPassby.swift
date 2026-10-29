import Foundation

/// `nearbyShare` mode:passby(すれ違い通信・#425)の純粋ロジック。通信(MultipeerConnectivity / NearbyInteraction)はアプリ本体。
///
/// - 送る: `fields` に明示した保存 field だけ。hidden(#262)・photo・video・ref・refs・computed は送らない
/// - 量: 1 回 `limit` 件(既定 1・最大 20 件)かつ JSON で 16 KB 以内
/// - 受け取り: 同じ相手(senderId)・同じ記録 id は 2 度追加しない。内容が変わっていれば更新する
public enum NearbyPassby {
    public static let version = 1
    public static let maxRecords = 20
    public static let maxBytes = 16 * 1024
    public static let defaultLimit = 1
    /// 同じ相手と続けて交換しない間隔(離れて戻ってきたら「また会った」)。
    public static let cooldown: TimeInterval = 10 * 60
    /// 同意文を変えたら上げる(もう一度同意を取る)。
    public static let consentVersion = 1
    public static let blockedTypes: Set<FieldType> = [.photo, .video, .ref, .refs]

    // MARK: - 設定

    public struct Config: Equatable, Hashable, Sendable {
        public var componentId: String
        public var entityId: String
        public var label: String
        public var fields: [String]
        public var limit: Int
        public var distance: Bool

        public init(componentId: String, entityId: String, label: String, fields: [String], limit: Int = NearbyPassby.defaultLimit, distance: Bool = false) {
            self.componentId = componentId
            self.entityId = entityId
            self.label = label
            self.fields = fields
            self.limit = Swift.min(Swift.max(limit, 1), NearbyPassby.maxRecords)
            self.distance = distance
        }

        /// passby の nearbyShare なら設定を返す。
        public init?(_ c: Component) {
            guard c.type == .nearbyShare, c.mode == "passby", let entity = c.entity else { return nil }
            self.init(componentId: c.id, entityId: entity, label: c.label ?? "すれ違い通信", fields: c.fields ?? [],
                      limit: c.limit ?? NearbyPassby.defaultLimit, distance: c.distance ?? false)
        }
    }

    /// 送ってよい field(明示 ∩ 保存 field − hidden − 写真・動画・ref)。DSL の並び順。
    public static func allowedFields(_ config: Config, in entity: Entity) -> [Field] {
        let listed = Set(config.fields)
        return entity.storedFields.filter { listed.contains($0.id) && !$0.hidden && !blockedTypes.contains($0.type) }
    }

    // MARK: - 送受信データ

    public struct Item: Codable, Equatable, Sendable {
        public var id: String
        public var data: [String: JSONValue]
        public init(id: String, data: [String: JSONValue]) { self.id = id; self.data = data }
    }

    /// 招待の context に載せる挨拶(小さく保つ)。`app` は appId のハッシュ(appId 自体は流さない)。
    public struct Hello: Codable, Equatable, Sendable {
        public var v: Int
        public var app: String
        public var senderId: String
        public var name: String
        public init(app: String, senderId: String, name: String) {
            v = NearbyPassby.version
            self.app = app
            self.senderId = senderId
            self.name = name
        }
    }

    public struct Payload: Codable, Equatable, Sendable {
        public var v: Int
        public var senderId: String
        public var name: String
        public var entityId: String
        public var records: [Item]

        public init(v: Int, senderId: String, name: String, entityId: String, records: [Item]) {
            self.v = v
            self.senderId = senderId
            self.name = name
            self.entityId = entityId
            self.records = records
        }
    }

    /// 同じアプリかを Bonjour の discoveryInfo / Hello で比べる印(FNV-1a 64bit・暗号用途ではない)。
    public static func appTag(_ appId: String) -> String {
        var h: UInt64 = 0xcbf2_9ce4_8422_2325
        for b in appId.utf8 { h = (h ^ UInt64(b)) &* 0x0000_0100_0000_01b3 }
        return String(h, radix: 16)
    }

    /// 送るデータ。自分の記録だけ(受け取った記録・他人の共有レコードは除く)を新しい順に `limit` 件、許可 field だけ。
    /// 16 KB を超える分は古い方から落とす。
    public static func makePayload(senderId: String, name: String, entity: Entity, config: Config,
                                   records: [Record], ledger: Ledger, me: String?) -> Payload {
        let allowed = Set(allowedFields(config, in: entity).map(\.id))
        let sorted = records.enumerated().sorted { a, b in
            let ka = a.element.createdAt ?? "", kb = b.element.createdAt ?? ""
            return ka == kb ? a.offset < b.offset : ka > kb
        }.map(\.element)
        var items: [Item] = []
        for r in sorted where items.count < config.limit {
            if let author = r.author, author != me { continue }
            if ledger.isReceived(r.data) { continue }
            let kept = r.data.filter { allowed.contains($0.key) && !$0.value.isNull }
            if !kept.isEmpty { items.append(Item(id: r.id, data: kept)) }
        }
        var p = Payload(v: version, senderId: senderId, name: name, entityId: entity.id, records: items)
        while !p.records.isEmpty, size(p) > maxBytes { p.records.removeLast() }
        return p
    }

    static func size(_ p: Payload) -> Int { (try? encode(p).count) ?? .max }

    /// 受け取った記録を自分の部品の設定に揃える: 許可 field 以外・型の合わない値・null を捨て、空の行と超過分を除く。
    public static func sanitize(_ payload: Payload, into entity: Entity, config: Config) -> [Item] {
        guard payload.entityId == entity.id else { return [] }
        let fields = Dictionary(allowedFields(config, in: entity).map { ($0.id, $0) }, uniquingKeysWith: { a, _ in a })
        return payload.records.prefix(maxRecords).compactMap { item in
            let data = item.data.filter { k, v in fields[k].map { NearbyShare.accepts(v, $0.type) } ?? false }
            return data.isEmpty || item.id.isEmpty ? nil : Item(id: item.id, data: data)
        }
    }

    // MARK: - 会った記録と重複除け

    public struct Encounter: Codable, Equatable, Identifiable, Sendable {
        public var id: String          // 相手の senderId
        public var name: String
        public var firstMet: Date
        public var lastMet: Date
        /// 会った回数(cooldown より間が空いたら +1)
        public var times: Int
        /// 受け取って追加・更新した記録の数(累計)
        public var records: Int

        public init(id: String, name: String, firstMet: Date, lastMet: Date, times: Int, records: Int) {
            self.id = id
            self.name = name
            self.firstMet = firstMet
            self.lastMet = lastMet
            self.times = times
            self.records = records
        }
    }

    public enum Plan: Equatable, Sendable {
        case create(key: String, data: [String: JSONValue])
        /// 前回受け取った内容(`old`)の記録を `new` に更新する
        case update(key: String, old: [String: JSONValue], new: [String: JSONValue])
        case skip(key: String)
    }

    /// 端末に保存する台帳(アプリごと)。受け取った記録の内容と、会った人の一覧。
    public struct Ledger: Codable, Equatable, Sendable {
        public static let maxEncounters = 200
        public static let maxReceived = 1000

        /// "senderId|recordId" → 最後に保存した内容
        public var received: [String: [String: JSONValue]] = [:]
        public var encounters: [Encounter] = []

        public init() {}

        public static func key(sender: String, id: String) -> String { sender + "|" + id }

        /// 受け取った記録か(保存した内容がそのまま残っている)。送り返さないための判定。
        public func isReceived(_ data: [String: JSONValue]) -> Bool {
            received.values.contains { Self.matches(data, $0) }
        }

        /// `record` が前回保存した `saved` の内容を全部持っているか。
        public static func matches(_ record: [String: JSONValue], _ saved: [String: JSONValue]) -> Bool {
            !saved.isEmpty && saved.allSatisfy { record[$0.key] == $0.value }
        }

        /// 前回保存した内容の記録を探す(ローカルの仮 id はサーバー保存で変わるので内容で探す)。
        public static func match(_ saved: [String: JSONValue], in records: [Record]) -> Record? {
            records.first { matches($0.data, saved) }
        }

        public func plan(_ items: [Item], from sender: String) -> [Plan] {
            items.map { item in
                let k = Self.key(sender: sender, id: item.id)
                guard let old = received[k] else { return .create(key: k, data: item.data) }
                return old == item.data ? .skip(key: k) : .update(key: k, old: old, new: item.data)
            }
        }

        public mutating func commit(_ key: String, _ data: [String: JSONValue]) {
            received[key] = data
            if received.count > Self.maxReceived, let drop = received.keys.sorted().first { received[drop] = nil }
        }

        /// 交換してよいか(同じ相手とは cooldown に 1 回)。
        public func shouldExchange(with sender: String, now: Date) -> Bool {
            guard let e = encounters.first(where: { $0.id == sender }) else { return true }
            return now.timeIntervalSince(e.lastMet) >= NearbyPassby.cooldown
        }

        /// 会った記録を付ける。新しい順に並べ、上限を超えた古い人は消す。
        public mutating func met(_ sender: String, name: String, at now: Date, records added: Int) {
            if let i = encounters.firstIndex(where: { $0.id == sender }) {
                var e = encounters.remove(at: i)
                if now.timeIntervalSince(e.lastMet) >= NearbyPassby.cooldown { e.times += 1 }
                e.name = name
                e.lastMet = now
                e.records += added
                encounters.insert(e, at: 0)
            } else {
                encounters.insert(Encounter(id: sender, name: name, firstMet: now, lastMet: now, times: 1, records: added), at: 0)
            }
            if encounters.count > Self.maxEncounters { encounters.removeLast(encounters.count - Self.maxEncounters) }
        }
    }

    // MARK: - 同意とスイッチ

    public enum ToggleOutcome: Equatable, Sendable {
        /// 初回: 何を誰に送るかの同意を先に出す
        case askConsent
        case start
        case stop
    }

    public static func toggle(on: Bool, consentedVersion: Int?) -> ToggleOutcome {
        guard on else { return .stop }
        return (consentedVersion ?? 0) >= consentVersion ? .start : .askConsent
    }

    /// 同意文。送る項目の名前を必ず並べる。
    public static func consentText(fieldNames: [String]) -> String {
        let list = fieldNames.isEmpty ? "(送る項目がありません)" : fieldNames.map { "「\($0)」" }.joined(separator: "")
        return "オンの間、近くで同じアプリを開いている人と自動でつながり、あなたの\(list)を交換します。"
            + "相手の記録はこの端末に追加されます。写真・動画や、それ以外の項目は送りません。"
            + "この画面を閉じるか、アプリを裏に回すと止まります。"
    }

    // MARK: - 距離・方向の表示

    public static func distanceText(_ meters: Float) -> String {
        meters < 10 ? String(format: "%.1f m", meters) : "\(Int(meters.rounded())) m"
    }

    /// 水平方向の角度(ラジアン・右が正)を言葉に。
    public static func directionText(azimuth: Float) -> String {
        let deg = azimuth * 180 / .pi
        switch deg {
        case -20...20: return "正面"
        case 20...70: return "右前"
        case -70 ... -20: return "左前"
        case 70...: return "右"
        default: return "左"
        }
    }

    // MARK: - 符号化

    public static func encode<T: Encodable>(_ value: T) throws -> Data {
        let e = JSONEncoder()
        e.outputFormatting = [.sortedKeys]
        e.dateEncodingStrategy = .iso8601
        return try e.encode(value)
    }

    public static func decode<T: Decodable>(_ type: T.Type, from data: Data) throws -> T {
        let d = JSONDecoder()
        d.dateDecodingStrategy = .iso8601
        return try d.decode(type, from: data)
    }
}
