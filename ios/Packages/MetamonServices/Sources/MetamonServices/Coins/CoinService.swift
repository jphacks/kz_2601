#if !APP_STORE_REVIEW // 審査用ビルドは App 内課金を持たない(#450)
import Foundation
import MetamonDSL

/// コインの口(#630)。appconfig が live の時は `HTTPCoinClient`、それ以外は nil(= コインの UI を出さない)。
public protocol CoinService: Sendable {
    /// 公開の設定(mode・パックの枚数・目安・最初の分・Pro の毎月の分と枠)。404 = まだ無いサーバー → `.disabled`
    func planConfig() async throws -> CoinPlanConfig
    /// 残高(出どころ別)・超えた後のコイン払い
    func wallet() async throws -> CoinWallet
    func ledger(cursor: String?) async throws -> CoinLedgerPage
    /// 消耗型の JWS を送って足す。同じ取引は何度送っても 1 回分(冪等)。返り値 = 足した後の残高
    func creditApplePurchase(signedTransaction: String) async throws -> CoinWallet
    func updateSettings(_ update: CoinSettingsUpdate) async throws -> CoinWallet
}

/// 402 のコインの断り(`coins_insufficient` / `allowance_exhausted` / `overflow_cap_reached`)。
public struct CoinDenial: Equatable, Sendable {
    public enum Kind: String, Sendable {
        /// 払う人のコインが足りない → コインのお店
        case coinsInsufficient = "coins_insufficient"
        /// サーバーの枠を使い切り、超えた後のコイン払いが OFF → 説明 + 設定
        case allowanceExhausted = "allowance_exhausted"
        /// 超えた後のコイン払いが月の上限に着いた → 説明 + 設定
        case overflowCapReached = "overflow_cap_reached"
    }

    public var kind: Kind
    public var required: Int?
    public var balance: Int?
    /// self = 自分・creator = 共有アプリの作成者が払う(参加者が買っても直らない)
    public var payer: String?

    public init(kind: Kind, required: Int? = nil, balance: Int? = nil, payer: String? = nil) {
        self.kind = kind
        self.required = required
        self.balance = balance
        self.payer = payer
    }

    public init?(_ error: Error) {
        guard let api = error as? APIError, api.status == 402,
              let code = api.code, let kind = Kind(rawValue: code),
              case .server(_, _, _, let details) = api else { return nil }
        self.init(kind: kind, required: CoinJSON.int(details["required"]), balance: CoinJSON.int(details["balance"]),
                  payer: details["payer"]?.stringValue)
    }

    /// 自分のコインで直せるか(共有アプリの作成者が払う時は false)
    public var payerIsSelf: Bool { payer == nil || payer == "self" }
}

/// `GET /v1/coins/config`・`GET /v1/me/coins`・`GET /v1/me/coins/ledger`・`POST /v1/me/coins/apple`・`PUT /v1/me/coins/settings`(appconfig)。
public struct HTTPCoinClient: CoinService {
    /// 公開の設定の置き場(契約の PR で変わったらここだけ直す)
    public static let configPath = "v1/coins/config"

    public let baseURL: URL
    public let token: any TokenProvider

    public init(baseURL: URL, token: any TokenProvider) {
        self.baseURL = baseURL
        self.token = token
    }

    private struct AppleBody: Encodable { var signedTransaction: String }

    public func planConfig() async throws -> CoinPlanConfig {
        do {
            let data = try await HTTP.send("GET", baseURL: baseURL, path: Self.configPath, body: nil, token: token, timeout: 15)
            return try CoinPlanConfig.decode(data)
        } catch let error as APIError where error.status == 404 {
            return .disabled
        }
    }

    public func wallet() async throws -> CoinWallet {
        try CoinWallet.decode(try await HTTP.send("GET", baseURL: baseURL, path: "v1/me/coins", body: nil, token: token, timeout: 15))
    }

    public func ledger(cursor: String?) async throws -> CoinLedgerPage {
        var path = "v1/me/coins/ledger"
        if let cursor, !cursor.isEmpty,
           let q = cursor.addingPercentEncoding(withAllowedCharacters: .alphanumerics.union(CharacterSet(charactersIn: "-._~"))) {
            path += "?cursor=\(q)"
        }
        return try CoinLedgerPage.decode(try await HTTP.send("GET", baseURL: baseURL, path: path, body: nil, token: token, timeout: 15))
    }

    public func creditApplePurchase(signedTransaction: String) async throws -> CoinWallet {
        let data = try await HTTP.send("POST", baseURL: baseURL, path: "v1/me/coins/apple",
                                       body: AppleBody(signedTransaction: signedTransaction), token: token, timeout: 20)
        return try CoinWallet.decodeCredit(data)
    }

    public func updateSettings(_ update: CoinSettingsUpdate) async throws -> CoinWallet {
        let data = try await HTTP.send("PUT", baseURL: baseURL, path: "v1/me/coins/settings", body: update, token: token, timeout: 15)
        return try CoinWallet.decode(data)
    }
}

/// 端末だけで動くコイン(UI テスト・`METAMON_COINS=1` のシミュレータ確認用)。数は見た目の確認用の仮の値。
public actor MockCoinService: CoinService {
    public private(set) var config: CoinPlanConfig
    private var current: CoinWallet
    private var entries: [CoinLedgerEntry] = []
    private var credited: Set<String> = []
    /// 送られた取引(テスト用)
    public private(set) var creditCalls: [String] = []
    /// true なら足すのに失敗する(取引を finish しないことの確認用)
    public var failCredit = false

    public init(config: CoinPlanConfig = MockCoinService.sampleConfig, wallet: CoinWallet = MockCoinService.sampleWallet) {
        self.config = config
        current = wallet
    }

    public func setFailCredit(_ fail: Bool) { failCredit = fail }

    public func planConfig() async throws -> CoinPlanConfig { config }
    public func wallet() async throws -> CoinWallet { current }
    public func ledger(cursor: String?) async throws -> CoinLedgerPage { CoinLedgerPage(entries: entries.reversed()) }

    public func creditApplePurchase(signedTransaction: String) async throws -> CoinWallet {
        creditCalls.append(signedTransaction)
        if failCredit { throw APIError.http(status: 503, body: "mock") }
        // モックの JWS は「mock:<productId>:<transactionId>」
        let parts = signedTransaction.split(separator: ":").map(String.init)
        let key = parts.count >= 3 ? parts[2] : signedTransaction
        guard !credited.contains(key) else { return current }
        credited.insert(key)
        let coins = parts.count >= 2 ? (config.pack(parts[1])?.totalCoins ?? 0) : 0
        current.balance.purchased += coins
        entries.append(CoinLedgerEntry(id: key, at: .now, kind: "purchase", delta: coins))
        return current
    }

    public func updateSettings(_ update: CoinSettingsUpdate) async throws -> CoinWallet {
        var o = current.overflow ?? CoinOverflow()
        o.enabled = update.overflow.enabled
        o.monthlyCap = update.overflow.monthlyCap
        current.overflow = o
        return current
    }

    public static let samplePackIDs = ["jp.kz2601.metamon.coins.small", "jp.kz2601.metamon.coins.medium", "jp.kz2601.metamon.coins.large"]

    // 見た目の確認用の仮の数(本番の量はサーバーの環境変数で決める・この repo には書かない)
    public static let sampleConfig = CoinPlanConfig(
        isEnabled: true,
        packs: [
            CoinPack(productId: samplePackIDs[0], coins: 100),
            CoinPack(productId: samplePackIDs[1], coins: 300),
            CoinPack(productId: samplePackIDs[2], coins: 1000),
        ],
        estimates: CoinEstimates(["generate": 30, "edit": 10, "chat": 2, "text": 1]),
        free: CoinPlanNumbers(oneTimeCoins: 50),
        pro: CoinPlanNumbers(monthlyCoins: 100, allowance: ["runs": 1000, "recipients": 500, "writes": 10000]))

    public static let sampleWallet = CoinWallet(
        mode: "enforce",
        balance: CoinBalance(purchased: 120, grant: 40, grantPeriodEnd: Calendar.current.date(byAdding: .day, value: 20, to: .now)),
        overflow: CoinOverflow(enabled: false, monthlyCap: 100, spent: 0))
}

/// コインの口(#630)。nil = コイン課金を使わない(今の利用者は何も変わらない)。
public enum CoinServiceKey: ServiceKey {
    public static var defaultValue: (any CoinService)? { nil }
}

extension Services {
    public var coins: (any CoinService)? {
        get { self[CoinServiceKey.self] }
        set { self[CoinServiceKey.self] = newValue }
    }
}
#endif
