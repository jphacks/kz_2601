#if METAMON_STG
import SwiftUI
import MetamonServices

/// 開発用メニュー: 課金と量の見え方(#2080)。端末の中だけで決まるものを切り替える(サーバーには何も書かない)。
/// サーバーが決める 402(pro_required・app_limit_reached・コイン)は「応答の作り物」(#2083)とサーバーの判定(#2085)。
final class DebugPlanSection: DebugMenuSection {
    override class var order: Int { 110 }
    override class var title: String { "課金と量" }
    override class var footer: String? {
        "端末を Pro として見てもサーバーは無料のまま判定します。その時に出る「端末は Pro なのにサーバーが知らない」案内(#2045)もこれで見られます。サーバーには何も書きません。"
    }
    @MainActor override class func makeBody() -> AnyView { AnyView(DebugPlanRows()) }
    @MainActor override class func reset() { DebugPlanState.shared.changed() }
}

/// 選べる値(メニューに出す順)。`value` = 保存する文字列(nil = 既定・消す)
enum DebugPlanChoices {
    static let plans: [(String, String?)] = [("実際のまま", nil), ("無料として見る", "free"), ("Pro として見る", "pro")]
    static let freeLimits: [(String, String?)] = [("既定", nil), ("0 回", "0"), ("1 回", "1"), ("3 回", "3")]
    static let stores: [(String, String?)] = [("既定", nil), ("StoreKit", "0"), ("端末のモック", "1")]
    static let coins: [(String, String?)] = [("サーバーのまま", nil), ("端末のモック", "1")]
    static let balances: [(String, String?)] = [("見本のまま", nil), ("0", "0"), ("少し(10)", "10"), ("たくさん(10000)", "10000")]
    static let usages: [(String, String?)] = [
        ("サーバーのまま", nil),
        ("上限の手前", "apps:9/10,records:4500/5000,photos:450/500"),
        ("上限", "apps:10/10,records:5000/5000,photos:500/500"),
        ("参加者として見る", "apps:3/10,records:4500/5000,photos:450/500,participant"),
    ]

    /// アプリの数の上限のシートに渡す作り物
    static func appLimit(canBuySlot: Bool) -> AppLimit {
        AppLimit(held: 10, limit: 10, canBuySlot: canBuySlot, slotCoins: canBuySlot ? 100 : nil,
                 balance: canBuySlot ? 120 : nil, upgrade: true)
    }
}

private struct PaywallItem: Identifiable {
    let id = UUID()
    var reason: PaywallReason
}

private struct DebugPlanRows: View {
    @Environment(AppRuntime.self) private var rt
    @State private var plan = DebugPlanState.shared
    @State private var paywall: PaywallItem?
    @State private var message: String?

    var body: some View {
        let _ = plan.observe()
        let gate = ProGate.shared
        Group {
            picker("プラン", DebugPlanKnobs.plan, DebugPlanChoices.plans, effect: .immediate, id: "plan")
            Toggle(isOn: Binding(get: { DebugPlanKnobs.forceEnforce },
                                 set: { plan.set(DebugPlanKnobs.iapEnforce, $0 ? "1" : nil) })) {
                DebugKnobLabel(title: "課金の制限を有効にする(「無効にする」より強い)", effect: .immediate,
                               envName: DebugPlanKnobs.iapEnforce)
            }
            .tint(Theme.Colors.brandA)
            .accessibilityIdentifier("debugMenu.plan.enforce")
            picker("1 日の無料作成の回数", DebugPlanKnobs.iapFreeLimit, DebugPlanChoices.freeLimits, effect: .immediate, id: "freeLimit")
            Button {
                gate.debugResetUsageToday()
                message = "今日の作成回数を 0 に戻しました"
            } label: {
                DebugKnobLabel(title: "今日の作成回数を 0 に戻す(今 \(gate.usedToday) 回 / 上限 \(gate.quota.dailyFreeLimit) 回)",
                               effect: .immediate)
            }
            .accessibilityIdentifier("debugMenu.plan.resetUsage")
            picker("課金の画面", DebugPlanKnobs.mockStore, DebugPlanChoices.stores, effect: .relaunch, id: "store")
            picker("コイン", DebugPlanKnobs.coins, DebugPlanChoices.coins, effect: .relaunch, id: "coins")
            if DebugPlanKnobs.coinsMocked {
                picker("コインのモックの残高", DebugPlanKnobs.coinsBalance, DebugPlanChoices.balances, effect: coinEffect, id: "coinsBalance")
                Toggle(isOn: Binding(get: { DebugKnobs.string(DebugPlanKnobs.coinsDisabled) == "1" },
                                     set: { setCoin(DebugPlanKnobs.coinsDisabled, $0 ? "1" : nil) })) {
                    DebugKnobLabel(title: "コイン課金が無効", effect: coinEffect, envName: DebugPlanKnobs.coinsDisabled)
                }
                .tint(Theme.Colors.brandA)
                .accessibilityIdentifier("debugMenu.plan.coinsDisabled")
                Button {
                    let balance = CoinGate.shared.store.wallet?.balance.available
                    let shown = CoinGate.shared.handle(CoinDenial(kind: .coinsInsufficient, required: 30, balance: balance, payer: "self"))
                    message = shown ? nil : "コインの画面は出ません(コイン課金が無効・まだ読み込み中)"
                } label: {
                    DebugKnobLabel(title: "コインが足りない時のお店を今出す", effect: .immediate)
                }
                .accessibilityIdentifier("debugMenu.plan.coinShop")
            }
            picker("使っている量", DebugPlanKnobs.mockUsage, DebugPlanChoices.usages, effect: .immediate, id: "usage")
            Button {
                presentAppLimit(canBuySlot: true)
            } label: {
                DebugKnobLabel(title: "アプリの数の上限のシートを今出す(枠を買える)", effect: .immediate)
            }
            .accessibilityIdentifier("debugMenu.plan.appLimitBuy")
            Button {
                presentAppLimit(canBuySlot: false)
            } label: {
                DebugKnobLabel(title: "アプリの数の上限のシートを今出す(枠を買えない)", effect: .immediate)
            }
            .accessibilityIdentifier("debugMenu.plan.appLimitNoBuy")
            paywallButton("課金画面を今出す(設定から)", .settings, id: "settings")
            paywallButton("課金画面を今出す(無料の回数を使い切った)", .limitReached, id: "limitReached")
            paywallButton("課金画面を今出す(有料の機能)", .paidFeature([]), id: "paidFeature")
            if let message {
                Text(verbatim: message)
                    .font(.footnote)
                    .foregroundStyle(.secondary)
                    .accessibilityIdentifier("debugMenu.plan.message")
            }
        }
        .appLimitSheets()
        .sheet(item: $paywall) { item in
            PaywallSheet(reason: item.reason)
        }
    }

    /// 起動時にモックを入れていればすぐ効く(入れ直す)。まだなら起動し直しが要る
    private var coinEffect: DebugKnobEffect {
        CoinGate.shared.store.service is MockCoinService ? .immediate : .relaunch
    }

    private func picker(_ title: String, _ name: String, _ choices: [(String, String?)], effect: DebugKnobEffect,
                        id: String) -> some View {
        let current = DebugKnobs.string(name)
        let selected = choices.firstIndex { $0.1 == current } ?? (current == nil ? 0 : -1)
        return Picker(selection: Binding(get: { selected }, set: { i in
            guard choices.indices.contains(i) else { return }
            set(name, choices[i].1, effect: effect)
        })) {
            ForEach(choices.indices, id: \.self) { i in Text(verbatim: choices[i].0).tag(i) }
            if selected < 0 { Text(verbatim: current ?? "-").tag(-1) }
        } label: {
            DebugKnobLabel(title: title, effect: effect, envName: name)
        }
        .accessibilityIdentifier("debugMenu.plan.\(id)")
    }

    private func set(_ name: String, _ value: String?, effect: DebugKnobEffect) {
        if name == DebugPlanKnobs.coinsBalance {
            setCoin(name, value)
            return
        }
        plan.set(name, value)
        if effect == .relaunch { DebugMenuState.shared.markNeedsRelaunch() }
        if name == DebugPlanKnobs.mockUsage {
            Task { await rt.refreshPlanUsage(force: true) }
        }
    }

    /// コインのモックの値。モックが入っていれば入れ直してすぐ効かせる
    private func setCoin(_ name: String, _ value: String?) {
        plan.set(name, value)
        let gate = CoinGate.shared
        guard gate.store.service is MockCoinService else {
            DebugMenuState.shared.markNeedsRelaunch()
            return
        }
        gate.store.service = DebugPlanKnobs.coinService()
        Task {
            await gate.store.refreshConfig()
            await gate.store.refreshWallet()
        }
    }

    private func presentAppLimit(canBuySlot: Bool) {
        let shown = AppLimitGate.shared.present(DebugPlanChoices.appLimit(canBuySlot: canBuySlot)) {}
        message = shown ? nil : "上限のシートを出せる画面がありません"
    }

    private func paywallButton(_ title: String, _ reason: PaywallReason, id: String) -> some View {
        Button {
            paywall = PaywallItem(reason: reason)
        } label: {
            DebugKnobLabel(title: title, effect: .immediate)
        }
        .accessibilityIdentifier("debugMenu.plan.paywall.\(id)")
    }
}
#endif
