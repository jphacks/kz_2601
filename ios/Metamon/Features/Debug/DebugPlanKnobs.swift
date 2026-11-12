#if DEBUG || METAMON_STG
import Foundation
import Observation
import MetamonServices

/// 課金と量のつまみ(#2080)。名前は起動時の環境変数と同じ。
/// stg の版(METAMON_STG)は環境変数 > 開発用メニューの保存値(`DebugKnobs`)、それ以外の DEBUG は環境変数だけ。
/// Release・Review の版には入らない(呼ぶ側も `#if DEBUG || METAMON_STG` の中)。
enum DebugPlanKnobs {
    /// "1" = 課金画面を端末のモックにする / "0" = StoreKit。起動時に決まる
    static let mockStore = "METAMON_MOCK_STORE"
    /// "1" = 課金の制限を有効にする(「課金の制限を無効にする」より強い)
    static let iapEnforce = "METAMON_IAP_ENFORCE"
    /// 1 日の無料作成の回数
    static let iapFreeLimit = "METAMON_IAP_FREE_LIMIT"
    /// "pro" / "free" = 端末だけ Pro / 無料として見る(`ProGate.isPro` の手前・`ProEntitlementStore` には触らない)
    static let plan = "METAMON_PLAN"
    /// "1" = コインを端末のモックにする。起動時に決まる
    static let coins = "METAMON_COINS"
    /// コインのモックの残高(買った分)
    static let coinsBalance = "METAMON_COINS_BALANCE"
    /// "1" = コインのモックでコイン課金を無効にする(`CoinPlanConfig.disabled`)
    static let coinsDisabled = "METAMON_COINS_DISABLED"
    /// 使っている量の見本(`apps:9/10,records:4200/5000,photos:10/500[,pro][,participant]`)
    static let mockUsage = "METAMON_MOCK_USAGE"

    static func string(_ name: String) -> String? {
        #if METAMON_STG
        DebugKnobs.string(name)
        #else
        ProcessInfo.processInfo.environment[name]
        #endif
    }

    /// 端末だけの Pro の上書き(nil = 実際のまま)
    static var proOverride: Bool? {
        switch string(plan)?.lowercased() {
        case "pro": true
        case "free": false
        default: nil
        }
    }

    static var forceEnforce: Bool { string(iapEnforce) == "1" }

    static var freeLimit: Int? { string(iapFreeLimit).flatMap(Int.init).map { max(0, $0) } }

    static var coinsMocked: Bool { string(coins) == "1" }

    /// コインのモック。残高と「無効」はつまみから
    static func coinService() -> MockCoinService {
        var wallet = MockCoinService.sampleWallet
        if let n = string(coinsBalance).flatMap(Int.init) {
            wallet.balance = CoinBalance(purchased: max(0, n))
        }
        let config = string(coinsDisabled) == "1" ? CoinPlanConfig.disabled : MockCoinService.sampleConfig
        return MockCoinService(config: config, wallet: wallet)
    }
}
#endif

#if METAMON_STG
/// メニューで変えた時に、つまみを読む画面(`ProGate.isPro` など)を描き直す印。値は `DebugKnobs` にある
@MainActor @Observable
final class DebugPlanState {
    static let shared = DebugPlanState()
    private(set) var revision = 0

    /// 読む側が呼ぶ(SwiftUI に依存を記録させる)
    func observe() { _ = revision }

    func set(_ name: String, _ value: String?) {
        DebugKnobs.set(name, value)
        revision += 1
    }

    func changed() { revision += 1 }
}
#endif
