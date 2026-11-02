// 審査用ビルド(APP_STORE_REVIEW)ではバイナリから外す(#507)。
#if !APP_STORE_REVIEW
import Foundation
import MetamonDSL

/// 仲間探しの API(docs/backend/openapi-appconfig.yaml `matching` タグ・#507)。
public struct HTTPMatchingClient: MatchingClient {
    public let baseURL: URL
    public let token: any TokenProvider

    public init(baseURL: URL, token: any TokenProvider) {
        self.baseURL = baseURL
        self.token = token
    }

    private struct SendBody: Encodable {
        var recordId: String
        var fromRecordId: String
        var ageConfirmed: Bool
    }
    private struct AcceptBody: Encodable { var ageConfirmed: Bool }
    private struct ListResponse: Decodable { var items: [MatchInterest]? }

    public func sendInterest(appId: String, recordId: String, fromRecordId: String, ageConfirmed: Bool) async throws -> MatchInterest {
        try JSONDecoder().decode(MatchInterest.self, from: try await request(
            "POST", "v1/apps/\(Self.escape(appId))/interests",
            body: SendBody(recordId: recordId, fromRecordId: fromRecordId, ageConfirmed: ageConfirmed)))
    }

    public func list(appId: String, box: MatchBox) async throws -> [MatchInterest] {
        let path = switch box {
        case .incoming, .outgoing: "v1/apps/\(Self.escape(appId))/interests?box=\(box.rawValue)"
        case .matches: "v1/apps/\(Self.escape(appId))/matches"
        }
        return try JSONDecoder().decode(ListResponse.self, from: try await request("GET", path)).items ?? []
    }

    public func accept(appId: String, uid: String, ageConfirmed: Bool) async throws -> MatchInterest {
        try JSONDecoder().decode(MatchInterest.self, from: try await request(
            "POST", "v1/apps/\(Self.escape(appId))/interests/\(Self.escape(uid))/accept", body: AcceptBody(ageConfirmed: ageConfirmed)))
    }

    public func decline(appId: String, uid: String) async throws {
        _ = try await request("DELETE", "v1/apps/\(Self.escape(appId))/interests/\(Self.escape(uid))")
    }

    public func unmatch(appId: String, uid: String) async throws {
        _ = try await request("DELETE", "v1/apps/\(Self.escape(appId))/matches/\(Self.escape(uid))")
    }

    static func escape(_ s: String) -> String {
        s.addingPercentEncoding(withAllowedCharacters: .alphanumerics.union(CharacterSet(charactersIn: "-_."))) ?? s
    }

    private func request(_ method: String, _ path: String, body: (any Encodable)? = nil) async throws -> Data {
        try await HTTP.send(method, baseURL: baseURL, path: path, body: body, token: token, timeout: 20)
    }
}
#endif
