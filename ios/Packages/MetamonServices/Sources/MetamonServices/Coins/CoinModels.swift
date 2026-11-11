// 審査用ビルド(APP_STORE_REVIEW・#450 G1)は App 内課金を持たない。コインもバイナリから外す。
#if !APP_STORE_REVIEW
import Foundation
import MetamonDSL

// コイン課金(#630)。契約は docs/backend/openapi-appconfig.yaml の coins(backend/internal/coins)。
// 読むのは JSONValue から寛容に(無い・型が違う項目は nil / 0 にして落ちない)。

/// 処理の種類(目安 `estimates` のキー・`COIN_ESTIMATE_<OP>`)。サーバーのキーは generate / edit / chat / text。
public enum CoinOperation: String, CaseIterable, Sendable {
    /// 新しく作る(`/v1/generate`)
    case generate
    /// 直す
    case edit
    /// 作る前の相談
    case chat
    /// アプリの中の AI(aiButton・aiChat の `/v1/text`)
    case text

    /// 目安を引くキー(大文字小文字・`_`・`-` は無視)。古い名前も受ける
    var keys: [String] {
        switch self {
        case .generate: ["generate", "create"]
        case .edit: ["edit"]
        case .chat: ["chat"]
        case .text: ["text", "aibutton", "aichat"]
        }
    }
}

/// 処理ごとの「約 N コイン」(`GET /v1/coins/config` の `estimates`)。
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
        for key in op.keys {
            if let n = values[Self.normalize(key)], n > 0 { return n }
        }
        return nil
    }

    public var isEmpty: Bool { values.isEmpty }

    init(json: JSONValue?) {
        var out: [String: Int] = [:]
        for (k, v) in json?.objectValue ?? [:] {
            // { "generate": 30 } と { "generate": { "coins": 30 } } の両方を受ける
            if let n = CoinJSON.int(v) ?? CoinJSON.int(v.objectValue?["coins"]) ?? CoinJSON.int(v.objectValue?["median"]) {
                out[k] = n
            }
        }
        self.init(out)
    }
}

/// 残高(出どころ別)。引く順番は grant(Pro の毎月の分・繰り越さない)→ initial(最初の分)→ purchased(買った分・期限なし)。
/// 買った分と毎月の分は画面で分ける(3.1.1)。
public struct CoinBalance: Equatable, Sendable {
    public var purchased: Int
    public var grant: Int
    public var grantPeriodEnd: Date?
    public var initial: Int
    /// 実行中の呼び出しが押さえている分(使える量には入らない)
    public var reserved: Int

    public init(purchased: Int, grant: Int = 0, grantPeriodEnd: Date? = nil, initial: Int = 0, reserved: Int = 0) {
        self.purchased = purchased
        self.grant = grant
        self.grantPeriodEnd = grantPeriodEnd
        self.initial = initial
        self.reserved = reserved
    }

    /// 新しい呼び出しに使える量(画面に出す残高)
    public var available: Int { purchased + grant + initial }
    /// available + reserved(履歴の合計と一致)
    public var total: Int { available + reserved }
}

/// 枠を超えた後のコイン払い(既定 OFF)と本人が決めた月の上限(`PUT /v1/me/coins/settings`)。
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

/// `GET /v1/me/coins`(`PUT /v1/me/coins/settings` の応答・`POST /v1/me/coins/apple` の `wallet` も同じ形)。
public struct CoinWallet: Equatable, Sendable {
    /// `COIN_BILLING_MODE`(enforce / log / off)。無ければ nil
    public var mode: String?
    public var balance: CoinBalance
    /// サーバーの枠(月)の使った量。サーバーはまだ返さない(#630 の 3 段目)ので、返るまで空 = 出さない
    public var allowance: [String: UsageMeter]
    /// nil = サーバーが返さない(設定の UI を出さない)
    public var overflow: CoinOverflow?
    /// 財布に目安があれば使う(無ければ公開の設定の目安)
    public var estimates: CoinEstimates
    /// コインで買った持てるアプリの枠(期限なし・#630)
    public var appSlots: Int

    public init(mode: String? = nil, balance: CoinBalance, allowance: [String: UsageMeter] = [:], overflow: CoinOverflow? = nil,
                estimates: CoinEstimates = .init(), appSlots: Int = 0) {
        self.mode = mode
        self.balance = balance
        self.allowance = allowance
        self.overflow = overflow
        self.estimates = estimates
        self.appSlots = appSlots
    }

    public static func decode(_ data: Data) throws -> CoinWallet {
        let root = try JSONDecoder().decode(JSONValue.self, from: data)
        guard let o = root.objectValue else { throw CoinDecodeError.notAnObject }
        return CoinWallet(object: o)
    }

    /// `POST /v1/me/coins/apple` の応答(`{credited, transactionId, productId, coins, wallet}`)から財布を読む。
    /// `wallet` が無ければ上に直に並ぶ形として読む。
    public static func decodeCredit(_ data: Data) throws -> CoinWallet {
        let root = try JSONDecoder().decode(JSONValue.self, from: data)
        guard let o = root.objectValue else { throw CoinDecodeError.notAnObject }
        return CoinWallet(object: o["wallet"]?.objectValue ?? o)
    }

    init(object o: [String: JSONValue]) {
        // 残高は上に直に並ぶ形(サーバー)と `balance: {...}` の両方を受ける
        let b = o["balance"]?.objectValue ?? o
        let balance = CoinBalance(
            purchased: CoinJSON.int(b["purchased"]) ?? 0,
            grant: CoinJSON.int(b["grant"]) ?? 0,
            grantPeriodEnd: CoinJSON.date(b["grantPeriodEnd"]),
            initial: CoinJSON.int(b["initial"]) ?? 0,
            reserved: CoinJSON.int(b["reserved"]) ?? 0)
        var allowance: [String: UsageMeter] = [:]
        for (k, v) in o["allowance"]?.objectValue ?? [:] {
            guard let m = v.objectValue, let used = CoinJSON.int(m["used"]) else { continue }
            allowance[k] = UsageMeter(used: used, limit: CoinJSON.int(m["limit"]), remaining: CoinJSON.int(m["remaining"]))
        }
        let overflow = o["overflow"]?.objectValue.map { ov in
            CoinOverflow(
                enabled: ov["enabled"]?.boolValue ?? false,
                monthlyCap: CoinJSON.int(ov["monthlyCap"]),
                spent: CoinJSON.int(ov["spent"]) ?? 0)
        }
        self.init(mode: o["mode"]?.stringValue, balance: balance, allowance: allowance, overflow: overflow,
                  estimates: CoinEstimates(json: o["estimates"]), appSlots: max(0, CoinJSON.int(o["appSlots"]) ?? 0))
    }
}

/// `POST /v1/me/coins/slot` の応答(`{bought, appSlots, coins, wallet}`)。
public struct CoinSlotPurchase: Equatable, Sendable {
    /// false = この requestId は買い済み(何も引いていない)
    public var bought: Bool
    public var appSlots: Int
    public var coins: Int
    public var wallet: CoinWallet

    public init(bought: Bool, appSlots: Int, coins: Int, wallet: CoinWallet) {
        self.bought = bought
        self.appSlots = appSlots
        self.coins = coins
        self.wallet = wallet
    }

    public static func decode(_ data: Data) throws -> CoinSlotPurchase {
        let root = try JSONDecoder().decode(JSONValue.self, from: data)
        guard let o = root.objectValue else { throw CoinDecodeError.notAnObject }
        return CoinSlotPurchase(bought: o["bought"]?.boolValue ?? false, appSlots: CoinJSON.int(o["appSlots"]) ?? 0,
                                coins: CoinJSON.int(o["coins"]) ?? 0, wallet: CoinWallet(object: o["wallet"]?.objectValue ?? [:]))
    }
}

/// 履歴の 1 行(`GET /v1/me/coins/ledger` の `entries`)。`delta` は増えれば正・引けば負(サーバーの `amount`)。
public struct CoinLedgerEntry: Equatable, Sendable, Identifiable {
    public var id: String
    public var at: Date?
    /// grant_initial / grant_monthly / grant_expire / purchase / spend_ai / spend_overflow /
    /// refund_failed / refund_apple / adjust(知らない種類もそのまま持つ)
    public var kind: String
    /// spend_ai の処理(generate / edit / chat / text)。無ければ nil
    public var operation: String?
    public var appId: String?
    public var delta: Int

    public init(id: String, at: Date?, kind: String, operation: String? = nil, appId: String? = nil, delta: Int) {
        self.id = id
        self.at = at
        self.kind = kind
        self.operation = operation
        self.appId = appId
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
        let items = o["entries"]?.arrayValue ?? o["items"]?.arrayValue ?? root.arrayValue ?? []
        var entries: [CoinLedgerEntry] = []
        for (i, item) in items.enumerated() {
            // 1 行読めなくても他の行は出す
            guard let e = item.objectValue,
                  let delta = CoinJSON.int(e["amount"]) ?? CoinJSON.int(e["delta"]) else { continue }
            let at = CoinJSON.date(e["at"])
            entries.append(CoinLedgerEntry(
                id: e["id"]?.stringValue ?? "\(i)-\(at?.timeIntervalSince1970 ?? 0)",
                at: at,
                kind: e["kind"]?.stringValue ?? "",
                operation: e["op"]?.stringValue,
                appId: e["appId"]?.stringValue,
                delta: delta))
        }
        let cursor = o["nextCursor"]?.stringValue
        return CoinLedgerPage(entries: entries, nextCursor: (cursor?.isEmpty ?? true) ? nil : cursor)
    }
}

/// コインのパック(消耗型)。値段は StoreKit から、枚数はサーバーの設定(`COIN_PACKS`)から(どちらもアプリに持たない)。
public struct CoinPack: Equatable, Sendable, Identifiable {
    public var productId: String
    public var coins: Int
    /// おまけの枚数。サーバーはまだ返さない(`COIN_PACKS` は productId=coins だけ)ので 0 = 印を出さない
    public var bonusCoins: Int

    public init(productId: String, coins: Int, bonusCoins: Int = 0) {
        self.productId = productId
        self.coins = coins
        self.bonusCoins = bonusCoins
    }

    public var id: String { productId }
    public var totalCoins: Int { coins + bonusCoins }
}

/// プランごとの数(表示用)。量は環境変数でサーバーが持つ。無い・0 のものは出さない。
public struct CoinPlanNumbers: Equatable, Sendable {
    /// 毎月のコイン(Pro・`proMonthly`。その月だけ)
    public var monthlyCoins: Int?
    /// 最初に 1 回だけもらえるコイン(サインインしたアカウント・`initialGrant`)
    public var oneTimeCoins: Int?
    /// サーバーの枠(月)。キー = runs / recipients / writes / reads
    public var allowance: [String: Int]

    public init(monthlyCoins: Int? = nil, oneTimeCoins: Int? = nil, allowance: [String: Int] = [:]) {
        self.monthlyCoins = monthlyCoins
        self.oneTimeCoins = oneTimeCoins
        self.allowance = allowance
    }

    static func allowance(_ json: JSONValue?) -> [String: Int] {
        var out: [String: Int] = [:]
        for (k, v) in json?.objectValue ?? [:] {
            if let n = CoinJSON.int(v) ?? CoinJSON.int(v.objectValue?["limit"]), n > 0 { out[k] = n }
        }
        return out
    }
}

/// 公開の設定(`GET /v1/coins/config`・認証不要)。コイン課金を出すかと、表示に使う数。
/// 404・読めない時は「出さない」(今の利用者は何も変わらない)。
public struct CoinPlanConfig: Equatable, Sendable {
    /// コインの UI を出すか(`mode == "enforce"` の時だけ。off / log では出さない)
    public var isEnabled: Bool
    public var packs: [CoinPack]
    public var estimates: CoinEstimates
    /// 無料: 最初のコイン(`initialGrant`)
    public var free: CoinPlanNumbers
    /// Pro: 毎月のコイン(`proMonthly`)と月の枠(`allowance`)
    public var pro: CoinPlanNumbers
    /// 超えた後のコイン払いの 1 まとまり(コイン数・実行の回数)。サーバーはまだ返さない(3 段目)ので nil = 出さない
    public var overflowBlockCoins: Int?
    public var overflowBlockRuns: Int?
    /// 持てるアプリの枠 1 つのコイン(`appSlotCoins`・#630)。nil = 枠を買えない
    public var appSlotCoins: Int?

    public init(isEnabled: Bool, packs: [CoinPack] = [], estimates: CoinEstimates = .init(),
                free: CoinPlanNumbers = .init(), pro: CoinPlanNumbers = .init(),
                overflowBlockCoins: Int? = nil, overflowBlockRuns: Int? = nil, appSlotCoins: Int? = nil) {
        self.appSlotCoins = appSlotCoins
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
        let positive = { (v: JSONValue?) -> Int? in CoinJSON.int(v).flatMap { $0 > 0 ? $0 : nil } }
        let ov = o["overflow"]?.objectValue ?? [:]
        return CoinPlanConfig(
            isEnabled: enabled,
            packs: packs,
            estimates: CoinEstimates(json: o["estimates"]),
            free: CoinPlanNumbers(oneTimeCoins: positive(o["initialGrant"])),
            pro: CoinPlanNumbers(monthlyCoins: positive(o["proMonthly"]), allowance: CoinPlanNumbers.allowance(o["allowance"])),
            overflowBlockCoins: positive(ov["blockCoins"]),
            overflowBlockRuns: positive(ov["blockRuns"]),
            appSlotCoins: positive(o["appSlotCoins"]))
    }
}

/// `PUT /v1/me/coins/settings` の本文(`{"overflow": {"enabled", "monthlyCap"}}`)。
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
        case .number(let n)? where n.isFinite && abs(n) < 9e15: return Int(n.rounded())
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
