import Foundation

/// `nearbyShare`(#36)の送受信データと取り込み判定。通信(MultipeerConnectivity)はアプリ本体、ここは純粋ロジックだけ。
///
/// - 招待時: `Header` を invitation context に載せる(受信側は接続前に「誰から・何が・何件」と取り込めるかを判定できる)
/// - 接続後: `Payload`(Header + レコード)を JSON で 1 回送り、受信側は `Ack` を返す
/// - 未知のキーは JSONDecoder が無視し、レコード内の未知フィールドは `sanitize` で捨てる
public enum NearbyShare {
    public static let version = 1
    /// 一覧画面から送る最大件数(新しい順)。
    public static let maxSendRecords = 50
    /// 受信側が取り込む最大件数(壊れた/悪意ある相手から大量に入れられないように)。
    public static let maxReceiveRecords = 200
    /// 送るデータ(JSON)の上限。超える分は古い方から落とす。
    public static let maxSendBytes = 512 * 1024
    /// 受け取るデータの上限(送る側の上限 + 余裕)。超えたら読まずに捨てる。
    public static let maxReceiveBytes = 1024 * 1024
    /// 招待に載る Header の上限。
    public static let maxHeaderBytes = 16 * 1024
    /// 受け取りの結果(Ack)の上限。
    public static let maxAckBytes = 4 * 1024

    public struct FieldSignature: Codable, Hashable, Sendable {
        public var id: String
        public var type: String
        public init(id: String, type: String) { self.id = id; self.type = type }
    }

    /// 招待に載せる要約(小さく保つ)。
    public struct Header: Codable, Equatable, Sendable {
        public var v: Int
        public var sender: String
        public var appId: String
        public var appName: String
        public var entityId: String
        public var entityName: String
        public var fields: [FieldSignature]
        public var count: Int
        /// 曲のリクエスト(`musicPlayer.allowRequests`・#412)。nil = レコードの共有。古い端末はこのキーを無視する
        public var song: MusicSong?

        public init(sender: String, appId: String, appName: String, entity: Entity, count: Int) {
            v = NearbyShare.version
            self.sender = sender
            self.appId = appId
            self.appName = appName
            entityId = entity.id
            entityName = entity.name
            fields = NearbyShare.signature(of: entity)
            self.count = count
        }

        /// 曲のリクエスト 1 件(レコードは送らない・曲は招待の context だけで判定できる)。
        public init(sender: String, appId: String, appName: String, song: MusicSong) {
            v = NearbyShare.version
            self.sender = sender
            self.appId = appId
            self.appName = appName
            entityId = NearbyShare.songEntityId
            entityName = "曲のリクエスト"
            fields = []
            count = 1
            self.song = song
        }
    }

    public struct Payload: Codable, Equatable, Sendable {
        public var header: Header
        public var records: [[String: JSONValue]]

        /// 送り主の名前を差し替えたもの(送る直前に今の MCPeerID の名前へ揃える。シートを開いた後に「名前を出さない」を切り替えても合う)。
        public func sent(as sender: String) -> Payload {
            var p = self
            p.header.sender = sender
            return p
        }
    }
}

extension NearbyShare.Header {
    /// 受け取った Header を上限に揃える。id・項目一覧が長すぎるものは nil。名前は切り詰め、空なら `NearbyLimits.anonymousName`。
    public func validated() -> Self? {
        guard NearbyLimits.isValidId(appId), NearbyLimits.isValidId(entityId), fields.count <= NearbyLimits.maxFields,
              fields.allSatisfy({ NearbyLimits.isValidId($0.id) && NearbyLimits.isValidId($0.type) }) else { return nil }
        var h = self
        h.sender = NearbyLimits.publicName(sender, anonymous: false)
        h.appName = NearbyLimits.clip(appName, NearbyLimits.maxLabelLength)
        h.entityName = NearbyLimits.clip(entityName, NearbyLimits.maxLabelLength)
        h.count = Swift.min(Swift.max(count, 0), NearbyShare.maxReceiveRecords)
        return h
    }
}

/// 近距離共有・すれ違い通信で受け取る値の上限と、相手に見せる名前(Android の `NearbyLimits` と同じ値)。
public enum NearbyLimits {
    /// 表示名の上限(文字数・MCPeerID の displayName は UTF-8 で 63 バイトまで)。
    public static let maxNameLength = 63
    /// senderId・記録の id・appId・entityId・項目 id の上限(文字数)。
    public static let maxIdLength = 128
    /// アプリ名・entity 名の上限(文字数)。
    public static let maxLabelLength = 100
    /// 受け取りの結果メッセージの上限(文字数)。
    public static let maxMessageLength = 200
    /// Header の項目一覧の上限(件数)。
    public static let maxFields = 200
    /// 名前を出さない時に表示名の代わりに送る決まった文言(言語に依らず同じ値を送り、表示する側で訳す)。
    public static let anonymousName = "近くの人"

    public static func clip(_ s: String, _ max: Int) -> String {
        String(s.trimmingCharacters(in: .whitespacesAndNewlines).prefix(max))
    }

    public static func isValidId(_ s: String) -> Bool { !s.isEmpty && s.count <= maxIdLength }

    /// 相手に送る名前。匿名・空なら `anonymousName`。
    public static func publicName(_ name: String, anonymous: Bool) -> String {
        let n = clip(name, maxNameLength)
        return anonymous || n.isEmpty ? anonymousName : n
    }

    /// 近距離共有の MCPeerID と Header の送り主に使う名前。`publicName` を UTF-8 で 63 バイト以内に切る(MCPeerID の上限)。
    public static func peerName(_ name: String, anonymous: Bool) -> String {
        var n = publicName(name, anonymous: anonymous)
        while n.utf8.count > maxNameLength { n.removeLast() }
        return n
    }

    /// 受け取った名前を画面に出す形に(匿名の印は今の言語の「近くの人」に)。
    public static func displayName(_ received: String) -> String {
        let n = clip(received, maxNameLength)
        return n.isEmpty || n == anonymousName ? L("近くの人") : n
    }
}

extension NearbyShare {

    /// 受信側 → 送信側の結果。
    public struct Ack: Codable, Equatable, Sendable {
        public var ok: Bool
        public var added: Int
        public var message: String?
        public init(ok: Bool, added: Int, message: String? = nil) { self.ok = ok; self.added = added; self.message = message }
    }

    /// 取り込み判定の結果。
    public enum Decision: Equatable, Sendable {
        /// 取り込める。`sameApp` = appId 一致(ref フィールドもそのまま使える)
        case accept(entityId: String, sameApp: Bool)
        case reject(reason: String)
    }

    // MARK: - 送信

    public static func signature(of entity: Entity) -> [FieldSignature] {
        entity.storedFields.map { FieldSignature(id: $0.id, type: $0.type.rawValue) }
    }

    /// 送るレコードを作る。保存フィールド以外(computed・未知キー)と null は落とし、空になったものは送らない。
    public static func makePayload(sender: String, appId: String, appName: String, entity: Entity,
                                   records: [[String: JSONValue]]) -> Payload {
        let stored = Set(entity.storedFields.map(\.id))
        let rows = records.prefix(maxSendRecords).compactMap { data -> [String: JSONValue]? in
            let kept = data.filter { stored.contains($0.key) && !$0.value.isNull }
            return kept.isEmpty ? nil : kept
        }
        var p = Payload(header: Header(sender: sender, appId: appId, appName: appName, entity: entity, count: rows.count),
                        records: Array(rows))
        while !p.records.isEmpty, ((try? encode(p).count) ?? .max) > maxSendBytes {
            p.records.removeLast()
            p.header.count = p.records.count
        }
        return p
    }

    /// 一覧画面で送る範囲: 新しい順(createdAt 降順・無ければ元の順)に最大 `maxSendRecords` 件。
    public static func recordsToSend(from records: [Record]) -> [[String: JSONValue]] {
        let sorted = records.enumerated().sorted { a, b in
            let ka = a.element.createdAt ?? "", kb = b.element.createdAt ?? ""
            return ka == kb ? a.offset < b.offset : ka > kb
        }
        return sorted.prefix(maxSendRecords).map(\.element.data)
    }

    // MARK: - 受信

    /// 招待の context を読む。大きすぎる・上限を超える Header は nil(招待を断る)。
    public static func decodeHeader(_ data: Data) -> Header? {
        guard data.count <= maxHeaderBytes else { return nil }
        return (try? decode(Header.self, from: data))?.validated()
    }

    /// 届いた Payload を読む。全体のバイト数を先に確かめ、上限を超えたら読まずに捨てる。
    public static func decodePayload(_ data: Data) -> Payload? {
        guard data.count <= maxReceiveBytes, var p = try? decode(Payload.self, from: data),
              let header = p.header.validated(), p.records.count <= maxReceiveRecords else { return nil }
        p.header = header
        return p
    }

    /// 送信側が受け取る Ack を読む。
    public static func decodeAck(_ data: Data) -> Ack? {
        guard data.count <= maxAckBytes, var ack = try? decode(Ack.self, from: data) else { return nil }
        ack.added = Swift.min(Swift.max(ack.added, 0), maxReceiveRecords)
        ack.message = ack.message.map { NearbyLimits.clip($0, NearbyLimits.maxMessageLength) }
        return ack
    }

    /// 受け取れるか。同じ appId なら entity id が在れば OK。別アプリなら entity id と保存フィールド(id+型)の集合が一致する時だけ。
    public static func decide(_ header: Header, dsl: AppDSL, appId: String) -> Decision {
        guard header.v <= version else { return .reject(reason: L("相手のアプリが新しすぎるため受け取れません")) }
        if let song = header.song { return decideSong(song, dsl: dsl, sameApp: header.appId == appId) }
        guard let entity = dsl.entity(header.entityId) else {
            return .reject(reason: L("このアプリには「\(header.entityName)」がないため受け取れません"))
        }
        if header.appId == appId { return .accept(entityId: entity.id, sameApp: true) }
        guard Set(signature(of: entity)) == Set(header.fields) else {
            return .reject(reason: L("「\(header.entityName)」の項目が違うため受け取れません(同じアプリを開いてください)"))
        }
        return .accept(entityId: entity.id, sameApp: false)
    }

    /// 受け取ったレコードを自分の entity の形に揃える: 未知フィールド・型が合わない値・null を捨てる。
    /// 別アプリからの ref は相手端末のレコード id なので捨てる。空になった行は除く。
    public static func sanitize(_ records: [[String: JSONValue]], into entity: Entity, sameApp: Bool) -> [[String: JSONValue]] {
        let fields = Dictionary(entity.storedFields.map { ($0.id, $0) }, uniquingKeysWith: { a, _ in a })
        return records.prefix(maxReceiveRecords).compactMap { data in
            var out: [String: JSONValue] = [:]
            for (key, value) in data {
                guard let f = fields[key], accepts(value, f.type), !((f.type == .ref || f.type == .refs) && !sameApp) else { continue }
                out[key] = value
            }
            return out.isEmpty ? nil : out
        }
    }

    static func accepts(_ value: JSONValue, _ type: FieldType) -> Bool {
        switch (type, value) {
        case (.integer, .number), (.number, .number), (.currency, .number): true
        case (.boolean, .bool): true
        case (.string, .string), (.text, .string), (.date, .string), (.datetime, .string),
             (.enum, .string), (.photo, .string), (.video, .string), (.audio, .string), (.location, .string), (.ref, .string), (.refs, .array): true
        default: false
        }
    }

    // MARK: - 曲のリクエスト(#412)

    /// 曲のリクエストの Decision.accept に入れる印(entity ではない)。
    public static let songEntityId = "$musicRequest"

    /// 送る曲のリクエストを作る。曲名が無ければ nil。
    public static func makeSongRequest(_ song: MusicSong, sender: String, appId: String, appName: String) -> Payload? {
        guard let s = MusicRequestRules.sanitize(song) else { return nil }
        return Payload(header: Header(sender: sender, appId: appId, appName: appName, song: s), records: [])
    }

    /// 今のアプリが allowRequests の musicPlayer を使っている時だけ受け取る(同じアプリの別コピーでもよい)。
    static func decideSong(_ song: MusicSong, dsl: AppDSL, sameApp: Bool) -> Decision {
        guard dsl.usesMusicRequests else { return .reject(reason: L("このアプリは曲のリクエストを受け付けていません")) }
        guard MusicRequestRules.sanitize(song) != nil else { return .reject(reason: L("曲名のないリクエストは受け取れません")) }
        return .accept(entityId: songEntityId, sameApp: sameApp)
    }

    // MARK: - 符号化

    public static func encode<T: Encodable>(_ value: T) throws -> Data {
        let e = JSONEncoder()
        e.outputFormatting = [.sortedKeys]
        return try e.encode(value)
    }

    public static func decode<T: Decodable>(_ type: T.Type, from data: Data) throws -> T {
        try JSONDecoder().decode(type, from: data)
    }
}
