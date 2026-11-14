import Foundation
import MetamonDSL

/// 共有のレコードの書き込みを、アプリの決まりでサーバーが断った理由(#272 #273・`details.reason`)。
/// 「作った人だけが変更できます」(#249)や入力の不備とは別の文言を出すために見分ける。審査用の版でも起きうるので外さない。
public enum RecordWriteDenial: Sendable, Equatable {
    /// 403 `admins_only`: 管理者だけが入れる・変える・消せる。`field` は断られた項目(分かる時)
    case adminsOnly(field: String?)
    /// 409 `duplicate_record`: entity の unique が同じレコードがもうある。`fields` は unique の field id(`$author` を含みうる)
    case duplicate(fields: [String])

    public init?(_ error: Error) {
        guard let api = error as? APIError, case .server(let status, _, _, let details) = api else { return nil }
        switch (status, api.reason) {
        case (403, "admins_only"?):
            if case .string(let f)? = details["field"] { self = .adminsOnly(field: f) } else { self = .adminsOnly(field: nil) }
        case (409, "duplicate"?):
            var fields: [String] = []
            if case .array(let xs)? = details["fields"] {
                for x in xs {
                    if case .string(let s) = x { fields.append(s) }
                }
            }
            self = .duplicate(fields: fields)
        default:
            return nil
        }
    }
}
