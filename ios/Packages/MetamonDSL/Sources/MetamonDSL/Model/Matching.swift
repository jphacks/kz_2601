// 審査用ビルド(APP_STORE_REVIEW)は共有を外しているので、仲間探し(#507)も外す。
#if !APP_STORE_REVIEW
import Foundation

/// `matchList.show`(#507)。appconfig の届いた / 送った(`GET /interests?box=`)・つながった(`GET /matches`)に対応する。
public enum MatchBox: String, CaseIterable, Sendable {
    case incoming, outgoing, matches

    public init?(show: String?) {
        guard let show, let box = MatchBox(rawValue: show) else { return nil }
        self = box
    }
}

/// 相手 1 人とのつながりの状態(appconfig `MatchInterest`: docs/backend/openapi-appconfig.yaml の `matching` タグ)。
/// 断られた・ブロックされたことは返らない(送った側には pending のまま見える)。
public struct MatchInterest: Codable, Hashable, Identifiable, Sendable {
    /// 相手の uid
    public var uid: String
    /// 相手のプロフィールのレコード id(行はこのレコードで描く)
    public var profileRecordId: String?
    /// pending | matched
    public var status: String
    public var updatedAt: String?
    public var matchedAt: String?

    public var id: String { uid }
    public var isMatched: Bool { status == "matched" }

    public init(uid: String, profileRecordId: String? = nil, status: String = "pending", updatedAt: String? = nil, matchedAt: String? = nil) {
        self.uid = uid
        self.profileRecordId = profileRecordId
        self.status = status
        self.updatedAt = updatedAt
        self.matchedAt = matchedAt
    }

    private enum Keys: String, CodingKey { case uid, profileRecordId, status, updatedAt, matchedAt }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: Keys.self)
        uid = try c.decode(String.self, forKey: .uid)
        profileRecordId = try? c.decodeIfPresent(String.self, forKey: .profileRecordId)
        status = (try? c.decodeIfPresent(String.self, forKey: .status)) ?? "pending"
        updatedAt = try? c.decodeIfPresent(String.self, forKey: .updatedAt)
        matchedAt = try? c.decodeIfPresent(String.self, forKey: .matchedAt)
    }
}

/// プロフィールの一覧の行から見た、その人とのつながり(`list.item.interest` のボタンの出し分け)。
public enum InterestState: Equatable, Sendable {
    /// まだ何も無い → 「つながりたい」
    case none
    /// 送って返事待ち → 「送りました」
    case sent
    /// 相手から届いている → 「承認する」(押すとつながる)
    case incoming
    /// つながった → 「つながりました」
    case matched
}
#endif
