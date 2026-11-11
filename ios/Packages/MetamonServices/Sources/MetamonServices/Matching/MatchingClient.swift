// 審査用ビルド(APP_STORE_REVIEW)は共有を外しているので、仲間探し(#507)も外す。
#if !APP_STORE_REVIEW
import Foundation
import MetamonDSL

/// 仲間探しの API(appconfig の `matching` タグ・#507)。`ageConfirmed` は本人の「18 歳以上」の確認(年齢そのものは送らない)。
public protocol MatchingClient: Sendable {
    /// `POST /v1/apps/{appId}/interests`: 相手のプロフィールに「つながりたい」を送る。相手が送っていればその場で matched。
    func sendInterest(appId: String, recordId: String, fromRecordId: String, ageConfirmed: Bool) async throws -> MatchInterest
    /// `GET /v1/apps/{appId}/interests?box=` / `GET /v1/apps/{appId}/matches`
    func list(appId: String, box: MatchBox) async throws -> [MatchInterest]
    /// `POST /v1/apps/{appId}/interests/{uid}/accept`
    func accept(appId: String, uid: String, ageConfirmed: Bool) async throws -> MatchInterest
    /// `DELETE /v1/apps/{appId}/interests/{uid}`: 届いたものを見送る / 送ったものを取り下げる(冪等・相手に知らせない)
    func decline(appId: String, uid: String) async throws
    /// `DELETE /v1/apps/{appId}/matches/{uid}`
    func unmatch(appId: String, uid: String) async throws
    /// `GET /v1/me/age-confirmation`: サーバーに 18 歳以上の申告が記録されているか
    func ageConfirmation() async throws -> Bool
    /// `PUT /v1/me/age-confirmation` `{"ageConfirmed":true}`: 申告を記録する(冪等)
    func confirmAge() async throws
}

/// API の `details.reason`(400)。
public enum MatchingReason {
    /// 18 歳以上の確認が無い
    public static let ageUnconfirmed = "age_unconfirmed"
    /// 自分がブロックしている相手には送れない
    public static let blocked = "blocked"
}
#endif
