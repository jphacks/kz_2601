import Foundation
import MetamonDSL

/// 持てるアプリの数の上限(#630・402 `app_limit_reached`・openapi-appconfig.yaml の AppLimitReached)。
/// 新しく作る・複製・シェアリンクの取り込み・「探す」から入れる時に返る。持っているアプリ(`apps`)から 1 つ消す / 抜けるか、
/// 枠をコインで買えば(`canBuySlot`)、同じ操作をやり直せる。
public struct AppLimit: Sendable, Equatable {
    public static let code = "app_limit_reached"

    /// 持っているアプリの 1 つ(`details.apps`)
    public struct HeldApp: Sendable, Equatable, Identifiable {
        public var id: String
        public var name: String
        /// 自分で作った(削除)。false = 参加した共有のアプリ(抜ける)
        public var mine: Bool
        public var updatedAt: Date?

        public init(id: String, name: String, mine: Bool, updatedAt: Date? = nil) {
            self.id = id
            self.name = name
            self.mine = mine
            self.updatedAt = updatedAt
        }
    }

    /// 今持っている数
    public var held: Int
    /// 上限(プランの数 + 買った枠)
    public var limit: Int
    public var purchasedSlots: Int
    /// 今 1 つ買える(サーバーがコイン課金を有効にし、枠の値段がある)
    public var canBuySlot: Bool
    /// 1 枠のコイン(買えない時は nil)
    public var slotCoins: Int?
    /// 使えるコイン(サーバーが知っていれば)
    public var balance: Int?
    /// Pro にすると上限が上がる
    public var upgrade: Bool
    /// 持っているアプリ。nil = サーバーが返さなかった(端末の一覧を使う)
    public var apps: [HeldApp]?

    public init(held: Int, limit: Int, purchasedSlots: Int = 0, canBuySlot: Bool = false, slotCoins: Int? = nil,
                balance: Int? = nil, upgrade: Bool = false, apps: [HeldApp]? = nil) {
        self.held = held
        self.limit = limit
        self.purchasedSlots = purchasedSlots
        self.canBuySlot = canBuySlot
        self.slotCoins = slotCoins
        self.balance = balance
        self.upgrade = upgrade
        self.apps = apps
    }

    public init?(_ error: Error) {
        guard let api = error as? APIError, api.status == 402, api.code == Self.code,
              case .server(_, _, _, let d) = api else { return nil }
        let int = { (v: JSONValue?) -> Int? in
            if case .number(let n)? = v, n.isFinite { return Int(n) }
            return nil
        }
        let apps: [HeldApp]? = d["apps"]?.arrayValue.map { items in
            items.compactMap { item -> HeldApp? in
                guard let o = item.objectValue, let id = o["id"]?.stringValue, !id.isEmpty else { return nil }
                return HeldApp(id: id, name: o["name"]?.stringValue ?? "", mine: o["mine"]?.boolValue ?? true,
                               updatedAt: o["updatedAt"]?.stringValue.flatMap(AppSummary.parseDate))
            }
        }
        let coins = int(d["slotCoins"]).flatMap { $0 > 0 ? $0 : nil }
        self.init(held: int(d["held"]) ?? 0, limit: int(d["limit"]) ?? 0, purchasedSlots: int(d["purchasedSlots"]) ?? 0,
                  canBuySlot: (d["canBuySlot"]?.boolValue ?? false) && coins != nil, slotCoins: coins,
                  balance: int(d["balance"]), upgrade: d["upgrade"]?.boolValue ?? false, apps: apps)
    }
}
