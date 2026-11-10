// 審査用ビルド(APP_STORE_REVIEW・#450 G1)は App 内課金を持たない。コインもバイナリから外す。
#if !APP_STORE_REVIEW
import Foundation
import MetamonDSL

// コイン課金(#630・docs/decisions/630-token-billing.md「API とエラーコード」)。
// 契約はまだ固まっていないので、すべて JSONValue から寛容に読む(無い・型が違う項目は nil / 0 にして落ちない)。

/// 処理の種類(目安 `estimates` のキー)。サーバーのキーは create / edit / chat / aiButton / aiChat / recipe を想定し、
/// 大文字小文字・`_`・`-` の違いは無視して引く。
public enum CoinOperation: String, CaseIterable, Sendable {
    case create
    case edit
    case chat
    case aiButton
    case aiChat
    case recipe
}

/// 処理ごとの「約 N コイン」(サーバーが最近の中央値から出す)。
public struct CoinEstimates: Equatable, Sendable {
    public var values: [String: Int]

    public init(_ values: [String: Int] = [:]) {
        self.values = Dictionary(values.map { (Self.normalize($0.key), $0.value) }, uniquingKeysWith: { a, _ in a })
    }

    static func normalize(_ key: String) -> String {
        key.lowercased().replacingOccurrences(of: "_", with: "").replacingOccurrences(of: "-", with: "")
    }

    /// 無い・0 以下は nil(出さない)
    public func coins(for op: CoinOperation) -> Int? {
        guard let n = values[Self.normalize(op.rawValue)], n > 0 else { return nil }
        return n
    }

    public var isEmpty: Bool { values.isEmpty }

    init(json: JSONValue?) {
        var out: [String: Int] = [:]
        for (k, v) in json?.objectValue ?? [:] {
            // { "create": 30 } と { "create": { "coins": 30 } } の両方を受ける
            if let n = CoinJSON.int(v) ?? CoinJSON.int(v.objectValue?["coins"]) ?? CoinJSON.int(v.objectValue?["median"]) {
                out[k] = n
            }
        }
        self.init(out)
    }
}

/// 残高。買った分(期限なし)と Pro の毎月の分(その月だけ・繰り越さない)を画面で分ける(3.1.1)。
public struct CoinBalance: Equatable, Sendable {
    public var purchased: Int
    public var grant: Int
    public var grantPeriodEnd: Date?

    public init(purchased: Int, grant: Int = 0, grantPeriodEnd: Date? = nil) {
        self.purchased = purchased
        self.grant = grant
        self.grantPeriodEnd = grantPeriodEnd
    }

    public var total: Int { purchased + grant }
}

/// 枠を超えた後のコイン払い(既定 OFF)と本人が決めた月の上限。
public struct CoinOverflow: Equatable, Sendable {
    public var enabled: Bool
    /// 月のコインの上限。nil = まだ決めていない
    public var monthlyCap: Int?
    /// 今月コインで払った量
    public var spent: Int

    public init(enabled: Bool = false, monthlyCap: Int? = nil, spent: Int = 0) {
        self.enabled = enabled
        self.monthlyCap = monthlyCap
        self.spent = spent
    }
}

/// `GET /v1/me/coins`。
public struct CoinWallet: Equatable, Sendable {
    public var balance: CoinBalance
    /// サーバーの枠(月)。キー = runs / notifications / writes / reads など。上限 nil = 数えない
    public var allowance: [String: UsageMeter]
    public var overflow: CoinOverflow
    public var estimates: CoinEstimates

    public init(balance: CoinBalance, allowance: [String: UsageMeter] = [:], overflow: CoinOverflow = .init(),
                estimates: CoinEstimates = .init()) {
        self.balance = balance
        self.allowance = allowance
        self.overflow = overflow
        self.estimates = estimates
    }

    public static func decode(_ data: Data) throws -> CoinWallet {
        let root = try JSONDecoder().decode(JSONValue.self, from: data)
        guard let o = root.objectValue else { throw CoinDecodeError.notAnObject }
        // 残高は `balance: {purchased, grant, grantPeriodEnd}` と、上に直に並ぶ形の両方を受ける
        let b = o["balance"]?.objectValue ?? o
        let balance = CoinBalance(
            purchased: CoinJSON.int(b["purchased"]) ?? 0,
            grant: CoinJSON.int(b["grant"]) ?? 0,
            grantPeriodEnd: CoinJSON.date(b["grantPeriodEnd"]))
        var allowance: [String: UsageMeter] = [:]
        for (k, v) in o["allowance"]?.objectValue ?? [:] {
            guard let m = v.objectValue, let used = CoinJSON.int(m["used"]) else { continue }
            allowance[k] = UsageMeter(used: used, limit: CoinJSON.int(m["limit"]), remaining: CoinJSON.int(m["remaining"]))
        }
        let ov = o["overflow"]?.objectValue ?? [:]
        let overflow = CoinOverflow(
            enabled: ov["enabled"]?.boolValue ?? false,
            monthlyCap: CoinJSON.int(ov["monthlyCap"]),
            spent: CoinJSON.int(ov["spent"]) ?? CoinJSON.int(ov["spentThisMonth"]) ?? CoinJSON.int(ov["used"]) ?? 0)
        return CoinWallet(balance: balance, allowance: allowance, overflow: overflow, estimates: CoinEstimates(json: o["estimates"]))
    }
}

/// 履歴の 1 行(`GET /v1/me/coins/ledger`)。`delta` は増えれば正・引けば負。
public struct CoinLedgerEntry: Equatable, Sendable, Identifiable {
    public var id: String
    public var at: Date?
    /// purchase | grant | spend | refund | overflow | refund_apple など(知らない種類もそのまま出す)
    public var kind: String
    /// create / edit / chat など。無ければ nil
    public var operation: String?
    public var appName: String?
    public var delta: Int

    public init(id: String, at: Date?, kind: String, operation: String? = nil, appName: String? = nil, delta: Int) {
        self.id = id
        self.at = at
        self.kind = kind
        self.operation = operation
        self.appName = appName
        self.delta = delta
    }
}

public struct CoinLedgerPage: Equatable, Sendable {
    public var entries: [CoinLedgerEntry]
    public var nextCursor: String?

    public init(entries: [CoinLedgerEntry], nextCursor: String? = nil) {
        self.entries = entries
        self.nextCursor = nextCursor
    }

    public static func decode(_ data: Data) throws -> CoinLedgerPage {
        let root = try JSONDecoder().decode(JSONValue.self, from: data)
        let o = root.objectValue ?? [:]
        let items = o["items"]?.arrayValue ?? o["entries"]?.arrayValue ?? root.arrayValue ?? []
        var entries: [CoinLedgerEntry] = []
        for (i, item) in items.enumerated() {
            // 1 行読めなくても他の行は出す
            guard let e = item.objectValue,
                  let delta = CoinJSON.int(e["delta"]) ?? CoinJSON.int(e["amount"]) ?? CoinJSON.int(e["coins"]) else { continue }
            let at = CoinJSON.date(e["at"]) ?? CoinJSON.date(e["createdAt"])
            entries.append(CoinLedgerEntry(
                id: e["id"]?.stringValue ?? "\(i)-\(at?.timeIntervalSince1970 ?? 0)",
                at: at,
                kind: e["kind"]?.stringValue ?? e["type"]?.stringValue ?? "",
                operation: e["operation"]?.stringValue ?? e["op"]?.stringValue,
                appName: e["appName"]?.stringValue,
                delta: delta))
        }
        let cursor = o["nextCursor"]?.stringValue ?? o["cursor"]?.stringValue
        return CoinLedgerPage(entries: entries, nextCursor: (cursor?.isEmpty ?? true) ? nil : cursor)
    }
}

/// コインのパック(消耗型)。値段は StoreKit から、枚数はサーバーの設定から(どちらもアプリに持たない)。
public struct CoinPack: Equatable, Sendable, Identifiable {
    public var productId: String
    public var coins: Int
    /// おまけの枚数(0 なら印を出さない)
    public var bonusCoins: Int

    public init(productId: String, coins: Int, bonusCoins: Int = 0) {
        self.productId = productId
        self.coins = coins
        self.bonusCoins = bonusCoins
    }

    public var id: String { productId }
    public var totalCoins: Int { coins + bonusCoins }
}

/// プランごとの数(表示用)。量は環境変数でサーバーが持つ。無いものは出さない。
public struct CoinPlanNumbers: Equatable, Sendable {
    /// 毎月のコイン(Pro。その月だけ)
    public var monthlyCoins: Int?
    /// サーバーの枠(月)。キー = runs / notifications / writes / reads など
    public var allowance: [String: Int]

    public init(monthlyCoins: Int? = nil, allowance: [String: Int] = [:]) {
        self.monthlyCoins = monthlyCoins
        self.allowance = allowance
    }

    init(json: JSONValue?) {
        let o = json?.objectValue ?? [:]
        var allowance: [String: Int] = [:]
        for (k, v) in o["allowance"]?.objectValue ?? [:] {
            if let n = CoinJSON.int(v) ?? CoinJSON.int(v.objectValue?["limit"]) { allowance[k] = n }
        }
        self.init(monthlyCoins: CoinJSON.int(o["monthlyCoins"]), allowance: allowance)
    }
}

/// 公開の設定(`GET /v1/coins/config`・サインイン前でも読める想定)。コイン課金を出すかのフラグと、表示に使う数。
/// 404・読めない時は「出さない」(今の利用者は何も変わらない)。
public struct CoinPlanConfig: Equatable, Sendable {
    /// サーバーがコイン課金を有効にしているか(`enabled` か `mode == "enforce"`)。false なら UI を一切出さない
    public var isEnabled: Bool
    public var packs: [CoinPack]
    public var estimates: CoinEstimates
    public var free: CoinPlanNumbers
    public var pro: CoinPlanNumbers
    /// 超えた後のコイン払いの 1 まとまり(コイン数・実行の回数)
    public var overflowBlockCoins: Int?
    public var overflowBlockRuns: Int?

    public init(isEnabled: Bool, packs: [CoinPack] = [], estimates: CoinEstimates = .init(),
                free: CoinPlanNumbers = .init(), pro: CoinPlanNumbers = .init(),
                overflowBlockCoins: Int? = nil, overflowBlockRuns: Int? = nil) {
        self.isEnabled = isEnabled
        self.packs = packs
        self.estimates = estimates
        self.free = free
        self.pro = pro
        self.overflowBlockCoins = overflowBlockCoins
        self.overflowBlockRuns = overflowBlockRuns
    }

    public static let disabled = CoinPlanConfig(isEnabled: false)

    public func pack(_ productId: String) -> CoinPack? { packs.first { $0.productId == productId } }

    public static func decode(_ data: Data) throws -> CoinPlanConfig {
        let root = try JSONDecoder().decode(JSONValue.self, from: data)
        guard let o = root.objectValue else { throw CoinDecodeError.notAnObject }
        let mode = o["mode"]?.stringValue?.lowercased()
        let enabled = o["enabled"]?.boolValue ?? (mode == "enforce")
        var packs: [CoinPack] = []
        for item in o["packs"]?.arrayValue ?? [] {
            guard let p = item.objectValue, let id = p["productId"]?.stringValue, !id.isEmpty,
                  let coins = CoinJSON.int(p["coins"]), coins > 0 else { continue }
            packs.append(CoinPack(productId: id, coins: coins, bonusCoins: max(0, CoinJSON.int(p["bonusCoins"]) ?? 0)))
        }
        let plans = o["plans"]?.objectValue ?? [:]
        let ov = o["overflow"]?.objectValue ?? [:]
        return CoinPlanConfig(
            isEnabled: enabled,
            packs: packs,
            estimates: CoinEstimates(json: o["estimates"]),
            free: CoinPlanNumbers(json: plans["free"]),
            pro: CoinPlanNumbers(json: plans["pro"]),
            overflowBlockCoins: CoinJSON.int(ov["blockCoins"]),
            overflowBlockRuns: CoinJSON.int(ov["blockRuns"]))
    }
}

/// `PUT /v1/me/coins/settings` の本文。
public struct CoinSettingsUpdate: Encodable, Equatable, Sendable {
    public struct Overflow: Encodable, Equatable, Sendable {
        public var enabled: Bool
        public var monthlyCap: Int
    }

    public var overflow: Overflow

    public init(overflowEnabled: Bool, monthlyCap: Int) {
        overflow = Overflow(enabled: overflowEnabled, monthlyCap: max(0, monthlyCap))
    }
}

public enum CoinDecodeError: Error, Equatable {
    case notAnObject
}

/// JSONValue から寛容に数・日時を読む。
enum CoinJSON {
    static func int(_ v: JSONValue?) -> Int? {
        switch v {
        case .number(let n)? where n.isFinite: return Int(n.rounded())
        case .string(let s)?: return Int(s)
        default: return nil
        }
    }

    static func date(_ v: JSONValue?) -> Date? {
        guard let raw = v?.stringValue else { return nil }
        if let d = ISO8601DateFormatter().date(from: raw) { return d }
        let frac = ISO8601DateFormatter()
        frac.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return frac.date(from: raw)
    }
}
#endif
