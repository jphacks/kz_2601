// 審査用ビルド(APP_STORE_REVIEW)は外部の公開データ(feed・#348)を外す。
#if !APP_STORE_REVIEW
import Foundation

/// `feed` が読むもの(`POST /v1/feeds/{provider}`・#348)。URL は持たない: provider と、その型の決まったパラメータだけ。
public struct FeedQuery: Hashable, Sendable {
    public var provider: String
    public var region: String?
    public var feedId: String?
    /// diet: 国会会議録で探す語(部品の `query`)。
    public var q: String?
    public var limit: Int

    public init(provider: String, region: String? = nil, feedId: String? = nil, q: String? = nil, limit: Int = 5) {
        self.provider = provider
        self.region = region
        self.feedId = feedId
        self.q = q
        self.limit = limit
    }

    /// 部品から。provider が知らない値なら nil(描かずにプレースホルダ)。
    public init?(_ c: Component) {
        guard let p = c.provider, Self.providers.contains(p) else { return nil }
        if p == "diet", (c.query ?? "").trimmingCharacters(in: .whitespaces).isEmpty { return nil }
        self.init(provider: p, region: Self.regionProviders.contains(p) ? c.region : nil,
                  feedId: Self.feedIdProviders.contains(p) ? c.feedId : nil, q: p == "diet" ? c.query : nil,
                  limit: Swift.min(Swift.max(c.limit ?? 5, 1), 20))
    }

    public static let providers: Set<String> = ["weather", "holidays", "rss", "warnings", "pressure", "arxiv", "diet"]
    /// `feedId`(許可した id)を読む provider。
    public static let feedIdProviders: Set<String> = ["rss", "arxiv"]
    /// `region`(府県予報区のコード)を読む provider。
    public static let regionProviders: Set<String> = ["weather", "warnings", "pressure"]

    /// `POST v1/feeds/{provider}` の宛先。パラメータは本文(`Encodable`)で送り、URL に載せない。
    public var path: String { "v1/feeds/\(provider)" }
}

/// POST の本文: `{"region","feedId","q","limit"}`(無いものは送らない・provider は宛先に入る)。
extension FeedQuery: Encodable {
    enum BodyKeys: String, CodingKey { case region, feedId, q, limit }

    public func encode(to encoder: Encoder) throws {
        var c = encoder.container(keyedBy: BodyKeys.self)
        try c.encodeIfPresent(region, forKey: .region)
        try c.encodeIfPresent(feedId, forKey: .feedId)
        try c.encodeIfPresent(q, forKey: .q)
        try c.encode(limit, forKey: .limit)
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
    /// pressure: 24 時間以内に 6 hPa 以上下がる。
    public var drop: Bool?
    /// pressure: 行の中の 1 時間ごとの気圧(hPa)。
    public var series: [Double]?

    public var id: String { "\(date ?? "")|\(title)|\(url ?? "")" }

    public init(date: String? = nil, title: String, subtitle: String? = nil, icon: String? = nil, url: String? = nil,
                max: Double? = nil, min: Double? = nil, pop: Int? = nil, drop: Bool? = nil, series: [Double]? = nil) {
        self.date = date
        self.title = title
        self.subtitle = subtitle
        self.icon = icon
        self.url = url
        self.max = max
        self.min = min
        self.pop = pop
        self.drop = drop
        self.series = series
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
