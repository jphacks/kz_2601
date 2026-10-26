import Foundation

/// scope:shared の参加者(#254)まわりの 403。`details.reason` で見分ける。
/// #378 の「作った人だけが変更できます」(reason 無しの forbidden)とは別の文言を出すために使う。
/// 審査用の版でも records の 403 は起きうるので、ここは `APP_STORE_REVIEW` で外さない。
public enum SharedAccessDenial: String, Sendable, Equatable {
    /// 共有のデータに参加していない(作成者でも参加者でもない)
    case notMember = "not_member"
    /// import: リンクを送った人がもう参加していない
    case inviterLeft = "inviter_left"
    /// import: 参加者が上限(100 人)
    case membersFull = "members_full"

    public init?(_ error: Error) {
        guard let api = error as? APIError, api.status == 403, let reason = api.reason else { return nil }
        self.init(rawValue: reason)
    }
}

extension APIError {
    /// `details.reason`(403 の理由など)。
    public var reason: String? {
        if case .server(_, _, _, let d) = self, case .string(let r)? = d["reason"] { return r }
        return nil
    }
}
