// 審査用ビルド(APP_STORE_REVIEW・#245)ではバイナリから外す。
#if !APP_STORE_REVIEW
import Foundation

/// `POST /v1/apps/{appId}/share` の応答(#49)。`url` = `tailorit://import/<token>`、`webUrl` = ブラウザ用フォールバック(`/s/<token>`)。
public struct ShareInfo: Codable, Sendable, Equatable {
    public var token: String
    public var appId: String
    public var name: String
    public var dslVersion: Int
    public var url: String
    public var webUrl: String
    public var revoked: Bool?

    public init(token: String, appId: String, name: String, dslVersion: Int, url: String, webUrl: String, revoked: Bool? = nil) {
        self.token = token
        self.appId = appId
        self.name = name
        self.dslVersion = dslVersion
        self.url = url
        self.webUrl = webUrl
        self.revoked = revoked
    }
}

/// 「探す」の 1 件(`GET /v1/public-apps` の items・#50)。DSL 本文は含まない。
public struct PublicAppListing: Codable, Sendable, Equatable, Identifiable, Hashable {
    public var id: String
    public var name: String
    public var description: String
    public var tags: [String]
    public var authorName: String
    /// 使っている部品の type(カードのアイコン用)
    public var components: [String]
    public var installs: Int
    /// 自分が公開したものか
    public var mine: Bool
    /// 運営が非公開にした(#321)。自分の掲載にだけ付く
    public var hidden: Bool?
    /// 運営の理由(作者に見せる)
    public var hiddenReason: String?

    public init(id: String, name: String, description: String = "", tags: [String] = [], authorName: String = "",
                components: [String] = [], installs: Int = 0, mine: Bool = false, hidden: Bool? = nil, hiddenReason: String? = nil) {
        self.id = id
        self.name = name
        self.description = description
        self.tags = tags
        self.authorName = authorName
        self.components = components
        self.installs = installs
        self.mine = mine
        self.hidden = hidden
        self.hiddenReason = hiddenReason
    }
}

/// import / install の応答。`appId` に切り替えれば変身できる。
/// `joined` = 共有データ(scope: shared/public)を持つアプリなので元の appId に参加した(#93 の共有レコードが見える)。
/// `member` = scope:shared のデータの参加者になった(import だけ・#254)。
public struct InstalledApp: Decodable, Sendable, Equatable {
    public var appId: String
    public var name: String
    public var joined: Bool
    public var member: Bool

    enum CodingKeys: String, CodingKey { case id, name, joined, member }

    public init(appId: String, name: String, joined: Bool = false, member: Bool = false) {
        self.appId = appId
        self.name = name
        self.joined = joined
        self.member = member
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        appId = try c.decode(String.self, forKey: .id)
        name = try c.decodeIfPresent(String.self, forKey: .name) ?? ""
        joined = try c.decodeIfPresent(Bool.self, forKey: .joined) ?? false
        member = try c.decodeIfPresent(Bool.self, forKey: .member) ?? false
    }
}

/// シェアと「探す」(appconfig の share / public-apps API)。
public protocol SharingClient: Sendable {
    func share(appId: String) async throws -> ShareInfo
    func revokeShare(token: String) async throws
    func importShare(token: String) async throws -> InstalledApp

    func publish(appId: String, description: String, tags: [String], authorName: String?) async throws -> PublicAppListing
    func unpublish(appId: String) async throws
    /// `sort`: `popular`(installs 降順)/ `new`
    func searchPublicApps(query: String, sort: String) async throws -> [PublicAppListing]
    func install(publicAppId: String) async throws -> InstalledApp
}

/// `tailorit://import/<token>` からトークンを取り出す(それ以外は nil)。
public enum ShareURL {
    public static let scheme = "tailorit"

    public static func token(from url: URL) -> String? {
        guard url.scheme?.lowercased() == scheme, url.host?.lowercased() == "import" else { return nil }
        let token = url.pathComponents.filter { $0 != "/" }.first ?? ""
        let allowed = CharacterSet.alphanumerics.union(CharacterSet(charactersIn: "_-.:"))
        guard !token.isEmpty, token.count <= 128, token.unicodeScalars.allSatisfy(allowed.contains) else { return nil }
        return token
    }
}
#endif
