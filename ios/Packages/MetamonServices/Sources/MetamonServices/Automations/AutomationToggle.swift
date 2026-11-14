// 審査用ビルド(APP_STORE_REVIEW)では外す(#614)。
#if !APP_STORE_REVIEW
import Foundation

/// ルールのオン / オフを `PATCH /v1/apps/{appId}/dsl` の JSON Patch にする(#347)。
/// オフ = `enabled: false` を足す、オン = `enabled` を外す(省略 = オン)。
public enum AutomationToggle {
    public struct Op: Encodable, Equatable, Sendable {
        public var op: String
        public var path: String
        public var value: Bool?

        public init(op: String, path: String, value: Bool? = nil) {
            self.op = op
            self.path = path
            self.value = value
        }
    }

    public struct Body: Encodable, Equatable, Sendable {
        public var patch: [Op]
    }

    public enum Failure: Error, Equatable {
        /// 今の DSL にそのルールが無い(一覧が古い)
        case ruleNotFound
        /// `GET /dsl` の応答が読めない
        case badEnvelope
    }

    /// 今の DSL(`GET /dsl` の封筒)から version と serverRules を取り出す。
    public static func parse(envelope data: Data) throws -> (version: Int, serverRules: [[String: Any]]) {
        guard let obj = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
              let version = obj["version"] as? Int else { throw Failure.badEnvelope }
        let dsl = obj["dsl"] as? [String: Any]
        return (version, dsl?["serverRules"] as? [[String: Any]] ?? [])
    }

    /// id で位置を引いて ops を作る(一覧の並びに頼らない)。もうその状態なら空(PATCH しない)。
    public static func ops(ruleId: String, enabled: Bool, serverRules: [[String: Any]]) throws -> [Op] {
        guard let i = serverRules.firstIndex(where: { $0["id"] as? String == ruleId }) else { throw Failure.ruleNotFound }
        let path = "/serverRules/\(i)/enabled"
        let current = serverRules[i]["enabled"]
        if enabled {
            return current == nil ? [] : [Op(op: "remove", path: path)]
        }
        return (current as? Bool) == false ? [] : [Op(op: "add", path: path, value: false)]
    }

    /// 他で変わっていた(取り直して見せる): 412 / 409 / 404・ルールが見つからない。
    public static func isConflict(_ error: Error) -> Bool {
        if let f = error as? Failure { return f == .ruleNotFound }
        if let e = error as? APIError, let s = e.status { return [404, 409, 412].contains(s) }
        return false
    }
}
#endif
