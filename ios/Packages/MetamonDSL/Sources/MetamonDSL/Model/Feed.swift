// 審査用ビルド(APP_STORE_REVIEW)は外部の公開データ(feed・#348)を外す。
#if !APP_STORE_REVIEW
import Foundation

/// `feed` が読むもの(`GET /v1/feeds/{provider}`・#348)。URL は持たない: provider と、その型の決まったパラメータだけ。
public struct FeedQuery: Hashable, Sendable {
    public var provider: String
    public var region: String?
    public var feedId: String?
    public var limit: Int

    public init(provider: String, region: String? = nil, feedId: String? = nil, limit: Int = 5) {
        self.provider = provider
        self.region = region
        self.feedId = feedId
        self.limit = limit
    }

    /// 部品から。provider が知らない値なら nil(描かずにプレースホルダ)。
    public init?(_ c: Component) {
        guard let p = c.provider, Self.providers.contains(p) else { return nil }
        self.init(provider: p, region: p == "weather" ? c.region : nil, feedId: p == "rss" ? c.feedId : nil,
                  limit: Swift.min(Swift.max(c.limit ?? 5, 1), 20))
    }

    public static let providers: Set<String> = ["weather", "holidays", "rss"]

    /// `v1/feeds/{provider}?…`(値は英数字とハイフンだけなので、そのまま URL に入る)。
    public var path: String {
        var items = [URLQueryItem]()
        if let region { items.append(URLQueryItem(name: "region", value: region)) }
        if let feedId { items.append(URLQueryItem(name: "feedId", value: feedId)) }
        items.append(URLQueryItem(name: "limit", value: String(limit)))
        var c = URLComponents()
        c.path = "v1/feeds/\(provider)"
        c.queryItems = items
        return c.string ?? "v1/feeds/\(provider)"
    }
}

/// サーバーの答え。知らないキーは無視し、壊れた行だけ落とす。
public struct FeedResult: Decodable, Sendable, Equatable {
    public var provider: String
    public var title: String
    public var items: [FeedItem]
    public var source: FeedSource
    public var stale: Bool

    public init(provider: String, title: String, items: [FeedItem], source: FeedSource, stale: Bool = false) {
        self.provider = provider
        self.title = title
        self.items = items
        self.source = source
        self.stale = stale
    }

    enum CodingKeys: String, CodingKey { case provider, title, items, source, stale }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        provider = try c.decode(String.self, forKey: .provider)
        title = try c.decodeIfPresent(String.self, forKey: .title) ?? ""
        items = (try? c.decode(Lossy<FeedItem>.self, forKey: .items).wrappedValue) ?? []
        source = try c.decode(FeedSource.self, forKey: .source)
        stale = try c.decodeIfPresent(Bool.self, forKey: .stale) ?? false
    }
}

public struct FeedItem: Decodable, Sendable, Equatable, Identifiable {
    public var date: String?
    public var title: String
    public var subtitle: String?
    public var icon: String?
    public var url: String?
    public var max: Double?
    public var min: Double?
    public var pop: Int?

    public var id: String { "\(date ?? "")|\(title)|\(url ?? "")" }

    public init(date: String? = nil, title: String, subtitle: String? = nil, icon: String? = nil, url: String? = nil,
                max: Double? = nil, min: Double? = nil, pop: Int? = nil) {
        self.date = date
        self.title = title
        self.subtitle = subtitle
        self.icon = icon
        self.url = url
        self.max = max
        self.min = min
        self.pop = pop
    }

    /// 開ける記事のリンク(https だけ)。
    public var link: URL? {
        guard let url, let u = URL(string: url), u.scheme == "https", u.host() != nil else { return nil }
        return u
    }
}

/// 出典(公共データ利用規約の出典の記載)。一覧の下に必ず出す。
public struct FeedSource: Decodable, Sendable, Equatable {
    public var name: String
    public var url: String
    public var license: String

    public init(name: String, url: String, license: String) {
        self.name = name
        self.url = url
        self.license = license
    }
}
#endif
