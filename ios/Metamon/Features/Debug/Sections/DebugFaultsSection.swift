#if METAMON_STG
import SwiftUI
import UIKit
import MetamonDSL
import MetamonServices

/// 開発用メニュー: サーバーの応答の作り物(#2083)。断り(402 / 409 / 429 / 401 / 5xx)・圏外・生成の途中で切れるを端末の中で作る。
/// サーバーには何も書かない(一致した通信はサーバーへ行かない)。決まりはメモリだけ(起動し直すと消える)。
final class DebugFaultsSection: DebugMenuSection {
    override class var order: Int { 300 }
    override class var title: String { "サーバーの応答" }
    override class var footer: String? {
        "見本を押すと、次の通信からサーバーへ行かずに作り物を返します。「次の 1 回」は最初に効いてから 20 秒の間のアプリ自身の送り直し"
            + "(ストリーム → 通常の呼び方・401 / 429 の再送)にも効きます。入れている間は画面の右上に出ます(押すと全部やめる)。"
            + "置き換えのシートのアプリは存在しない id なので、削除・抜けるはサーバーで 404 になります。"
    }
    @MainActor override class func makeBody() -> AnyView {
        DebugFaultModel.shared.start()
        return AnyView(DebugFaultRows())
    }
    @MainActor override class func reset() { FaultInjection.removeAll() }
}

// MARK: - 見本

/// どの通信に効かせるか(見本の既定を上書きする)
enum DebugFaultTarget: String, CaseIterable, Identifiable {
    case preset, all, generate, ext, appList
    var id: String { rawValue }

    var label: String {
        switch self {
        case .preset: "見本の既定"
        case .all: "全部の通信"
        case .generate: "生成(v1/generate)"
        case .ext: "外の API(v1/apps/*/ext)"
        case .appList: "アプリの一覧(GET v1/apps)"
        }
    }

    var match: FaultMatch? {
        switch self {
        case .preset: nil
        case .all: .all
        case .generate: DebugFaultPreset.generate
        case .ext: DebugFaultPreset.ext
        case .appList: FaultMatch(path: "v1/apps", method: "GET")
        }
    }
}

/// メニューに並べる見本。本文はサーバー(backend/internal/httpx/codes.go と各ハンドラ)と同じ形
struct DebugFaultPreset: Identifiable {
    var id: String
    var group: String
    var title: String
    var match: FaultMatch
    var outcome: FaultOutcome
    var delay: TimeInterval = 0
    /// 「次の 1 回」の時に送り直しへも効かせる秒。生成の途中で切れるは 0(通常の呼び方は本物へ通して戻るのを見る)
    var followUp: TimeInterval = 20

    static let generate = FaultMatch(path: "v1/generate", method: "POST")
    static let ext = FaultMatch(path: "v1/apps/*/ext", method: "POST")
    static let extProvider = "fakeOauth"

    func rule(always: Bool, target: DebugFaultTarget) -> FaultRule {
        let m = target.match ?? match
        let suffix = target == .preset ? "" : "・\(target.label)"
        return FaultRule(label: title + suffix, match: m, outcome: outcome, delay: delay,
                         times: always ? .always : .once(followUp: followUp))
    }

    static let groups = ["課金・コイン(402)", "外の API", "混雑・上限・サインイン", "通信"]

    static let all: [DebugFaultPreset] = billing + external + busy + transport

    // MARK: 402

    private static let proFeatures: JSONValue = [["id": "sharedData", "title": "みんなで共有するデータ",
                                                  "reason": "ほかの人と同じデータを読み書きするため、サーバーの費用がかかります。"]]

    private static func heldApps() -> JSONValue {
        let t = ISO8601DateFormatter().string(from: Date(timeIntervalSinceNow: -86_400))
        return [["id": "debug-held-1", "name": "家計簿(作り物)", "mine": true, "updatedAt": .string(t)],
                ["id": "debug-held-2", "name": "読書メモ(作り物)", "mine": true, "updatedAt": .string(t)],
                ["id": "debug-held-3", "name": "サークルの出欠(作り物・参加)", "mine": false, "updatedAt": .string(t)]]
    }

    private static func appLimit(canBuy: Bool) -> FaultOutcome {
        var msg = "持てるアプリの数(3 個)に達しました。使わないアプリを削除するか、共有のアプリから抜けると、新しく作ったり参加したりできます。"
        if canBuy { msg += "コインで枠を 1 つ増やすこともできます。" }
        return .error(status: 402, code: "app_limit_reached", message: msg, details: [
            "resource": "apps", "plan": "free", "held": 3, "limit": 3, "base": 3, "purchasedSlots": 0,
            "canBuySlot": .bool(canBuy), "slotCoins": canBuy ? 100 : 0, "balance": canBuy ? 250 : 0, "upgrade": true,
            "apps": heldApps(),
        ])
    }

    static let billing: [DebugFaultPreset] = {
        let g = groups[0]
        return [
            DebugFaultPreset(id: "pro", group: g, title: "402 pro_required(自分の Pro)", match: generate,
                             outcome: .error(status: 402, code: "pro_required", message: "this needs Tailor It Pro: sharedData",
                                             details: ["features": proFeatures])),
            DebugFaultPreset(id: "proCreator", group: g, title: "402 pro_required(作った人の Pro)", match: generate,
                             outcome: .error(status: 402, code: "pro_required", message: "this needs Tailor It Pro: sharedData",
                                             details: ["features": proFeatures, "reason": "creator_needs_pro"])),
            DebugFaultPreset(id: "appLimitBuy", group: g, title: "402 app_limit_reached(枠を買える)", match: generate,
                             outcome: appLimit(canBuy: true)),
            DebugFaultPreset(id: "appLimitNoBuy", group: g, title: "402 app_limit_reached(枠を買えない)", match: generate,
                             outcome: appLimit(canBuy: false)),
            DebugFaultPreset(id: "coinsSelf", group: g, title: "402 coins_insufficient(自分が払う)", match: generate,
                             outcome: .error(status: 402, code: "coins_insufficient", message: "not enough coins",
                                             details: ["required": 5, "balance": 1, "payer": "self"])),
            DebugFaultPreset(id: "coinsCreator", group: g, title: "402 coins_insufficient(作った人が払う)", match: generate,
                             outcome: .error(status: 402, code: "coins_insufficient", message: "not enough coins",
                                             details: ["required": 5, "balance": 1, "payer": "creator"])),
            DebugFaultPreset(id: "allowance", group: g, title: "402 allowance_exhausted", match: generate,
                             outcome: .error(status: 402, code: "allowance_exhausted", message: "the monthly server allowance is used up")),
            DebugFaultPreset(id: "overflowCap", group: g, title: "402 overflow_cap_reached", match: generate,
                             outcome: .error(status: 402, code: "overflow_cap_reached", message: "the monthly coin cap for overflow is reached",
                                             details: ["cap": 500, "spent": 500])),
        ]
    }()

    // MARK: 外の API(メニューの「つないだサービス」→「試験用のサービスを呼ぶ」で確かめられる)

    static let external: [DebugFaultPreset] = {
        let g = groups[1], p = JSONValue.string(extProvider)
        return [
            DebugFaultPreset(id: "extNotConnected", group: g, title: "409 ext_not_connected", match: ext,
                             outcome: .error(status: 409, code: "ext_not_connected", message: "このサービスにつないでいません。",
                                             details: ["provider": p])),
            DebugFaultPreset(id: "extUnavailable", group: g, title: "503 ext_unavailable", match: ext,
                             outcome: .error(status: 503, code: "ext_unavailable", message: "このサービスは今は使えません。",
                                             details: ["provider": p])),
            DebugFaultPreset(id: "extBusy", group: g, title: "503 ext_busy", match: ext,
                             outcome: .error(status: 503, code: "ext_busy", message: "混み合っています。しばらくしてからもう一度試してください。",
                                             details: ["provider": p])),
            DebugFaultPreset(id: "extPro", group: g, title: "402 pro_required(外の API)", match: ext,
                             outcome: .error(status: 402, code: "pro_required", message: "this needs Tailor It Pro: externalCall")),
            DebugFaultPreset(id: "extCoins", group: g, title: "402 coins_insufficient(外の API)", match: ext,
                             outcome: .error(status: 402, code: "coins_insufficient", message: "not enough coins", details: ["payer": "self"])),
            DebugFaultPreset(id: "extRate", group: g, title: "429 rate_limited(外の API・Retry-After 30)", match: ext,
                             outcome: .error(status: 429, code: "rate_limited", message: "しばらくしてからもう一度試してください。",
                                             details: ["scope": "user", "provider": p, "retryAfterSec": 30],
                                             headers: ["Retry-After": "30"])),
        ]
    }()

    // MARK: 混雑・上限・サインイン

    private static func rateDetails(scope: String, tier: String) -> [String: JSONValue] {
        let reset = Date(timeIntervalSinceNow: 600)
        return ["scope": .string(scope), "tier": .string(tier), "limit": 20, "used": 20, "remaining": 0,
                "resetAt": .string(ISO8601DateFormatter().string(from: reset)), "retryAfterSec": 600]
    }

    static let busy: [DebugFaultPreset] = {
        let g = groups[2]
        return [
            DebugFaultPreset(id: "rateUser", group: g, title: "429 rate_limited(user・Retry-After 600 = 送り直さない)", match: generate,
                             outcome: .error(status: 429, code: "rate_limited", message: "今日の利用上限(20)に達しました。",
                                             details: rateDetails(scope: "user", tier: "signed_in"), headers: ["Retry-After": "600"])),
            DebugFaultPreset(id: "rateNoHeader", group: g, title: "429 rate_limited(ip・Retry-After 無し = 2 秒後に 1 回送り直す)",
                             match: generate,
                             outcome: .error(status: 429, code: "rate_limited",
                                             message: "このネットワークからの本日の利用上限に達しました。サインインすると続けて使えます。",
                                             details: rateDetails(scope: "ip", tier: "anonymous"))),
            DebugFaultPreset(id: "rateClaude", group: g, title: "429 rate_limited(scope 無し = 混雑)", match: generate,
                             outcome: .error(status: 429, code: "rate_limited", message: "Claude rate limit", headers: ["Retry-After": "60"])),
            DebugFaultPreset(id: "unauthenticated", group: g, title: "401 unauthenticated(サインインが切れた)", match: generate,
                             outcome: .error(status: 401, code: "unauthenticated", message: "unauthenticated")),
            DebugFaultPreset(id: "internal", group: g, title: "500 internal", match: generate,
                             outcome: .error(status: 500, code: "internal", message: "internal server error")),
            DebugFaultPreset(id: "unavailable", group: g, title: "503 upstream_unavailable(Retry-After 5)", match: generate,
                             outcome: .error(status: 503, code: "upstream_unavailable", message: "今は使えません。しばらくしてからもう一度試してください。",
                                             headers: ["Retry-After": "5"])),
        ]
    }()

    // MARK: 通信

    private static let streamFrames: [String] = [
        FaultOutcome.frame(event: "app", data: #"{"attempt":1,"name":"作り物のアプリ","tabs":["ホーム","記録"]}"#),
        FaultOutcome.frame(event: "component", data: #"{"attempt":1,"screen":"home","component":{"index":0,"type":"text","label":"見出し"}}"#),
        FaultOutcome.frame(event: "component", data: #"{"attempt":1,"screen":"home","component":{"index":1,"type":"list","label":"一覧"}}"#),
    ]

    static let transport: [DebugFaultPreset] = {
        let g = groups[3]
        return [
            DebugFaultPreset(id: "offline", group: g, title: "圏外(notConnectedToInternet)", match: .all, outcome: .offline),
            DebugFaultPreset(id: "slow", group: g, title: "とても遅い(30 秒待ってから本物へ)", match: generate,
                             outcome: .passThrough, delay: 30, followUp: 0),
            DebugFaultPreset(id: "streamCut", group: g, title: "生成の途中で切れる(3 件で切断 → 通常の呼び方へ)",
                             match: FaultMatch(path: "v1/generate", method: "POST", query: "stream=1"),
                             outcome: .events(frames: streamFrames, cut: true), followUp: 0),
            DebugFaultPreset(id: "streamError", group: g, title: "生成のストリームで error(rate_limited global)",
                             match: FaultMatch(path: "v1/generate", method: "POST", query: "stream=1"),
                             outcome: .events(frames: Array(streamFrames.prefix(1)) + [FaultOutcome.errorFrame(
                                status: 429, code: "rate_limited", message: "本日は利用が集中したため、新しい生成の受付を止めています。",
                                details: rateDetails(scope: "global", tier: "signed_in"))], cut: false)),
            DebugFaultPreset(id: "streamFailed", group: g, title: "生成のストリームで error(500 generation_failed)",
                             match: FaultMatch(path: "v1/generate", method: "POST", query: "stream=1"),
                             outcome: .events(frames: Array(streamFrames.prefix(1)) + [FaultOutcome.errorFrame(
                                status: 500, code: "generation_failed", message: "generation failed")], cut: false)),
        ]
    }()
}

// MARK: - 状態

/// 入っている決まり(メニューと右上の印の表示)
@MainActor @Observable
final class DebugFaultModel {
    static let shared = DebugFaultModel()
    private(set) var rules: [FaultRule] = []
    private var started = false

    func start() {
        guard !started else { return }
        started = true
        FaultInjection.onChange = { DispatchQueue.main.async { DebugFaultModel.shared.refresh() } }
        refresh()
    }

    func refresh() {
        rules = FaultInjection.rules
        DebugFaultBadge.update(count: rules.count)
    }
}

// MARK: - 行

private struct DebugFaultRows: View {
    @State private var model = DebugFaultModel.shared
    @State private var always = false
    @State private var target = DebugFaultTarget.preset

    var body: some View {
        if !model.rules.isEmpty {
            ForEach(model.rules) { rule in
                HStack {
                    VStack(alignment: .leading, spacing: 2) {
                        Text(verbatim: rule.label).font(.footnote)
                        Text(verbatim: Self.caption(rule)).font(.caption).foregroundStyle(.secondary)
                    }
                    Spacer()
                    Button { FaultInjection.remove(id: rule.id) } label: { Image(systemName: "xmark.circle.fill") }
                        .buttonStyle(.borderless)
                        .accessibilityLabel(Text(verbatim: "やめる"))
                }
                .accessibilityIdentifier("debugMenu.faults.active")
            }
            Button(role: .destructive) { FaultInjection.removeAll() } label: { Text(verbatim: "全部やめる") }
                .accessibilityIdentifier("debugMenu.faults.stopAll")
        }
        Picker(selection: $always) {
            Text(verbatim: "次の 1 回").tag(false)
            Text(verbatim: "ずっと").tag(true)
        } label: {
            DebugKnobLabel(title: "回数", effect: .immediate)
        }
        .accessibilityIdentifier("debugMenu.faults.times")
        Picker(selection: $target) {
            ForEach(DebugFaultTarget.allCases) { Text(verbatim: $0.label).tag($0) }
        } label: {
            Text(verbatim: "対象")
        }
        .accessibilityIdentifier("debugMenu.faults.target")
        ForEach(DebugFaultPreset.groups, id: \.self) { group in
            DisclosureGroup {
                ForEach(DebugFaultPreset.all.filter { $0.group == group }) { preset in
                    Button {
                        FaultInjection.add(preset.rule(always: always, target: target))
                    } label: {
                        Text(verbatim: preset.title).font(.footnote)
                    }
                    .accessibilityIdentifier("debugMenu.faults.\(preset.id)")
                }
            } label: {
                Text(verbatim: group)
            }
        }
    }

    private static func caption(_ rule: FaultRule) -> String {
        let times = switch rule.times {
        case .always: "ずっと"
        case .once: rule.hits == 0 ? "次の 1 回" : "次の 1 回(効いた・送り直しを待っている)"
        }
        return "\(times)・\(rule.hits) 回効いた"
    }
}

// MARK: - 右上の印

/// 作り物を入れている間だけ、画面の右上に小さく出す(押すと全部やめる)。ほかの画面のコードは触らないので、小さい窓を重ねる
@MainActor
enum DebugFaultBadge {
    private static var window: UIWindow?

    static func update(count: Int) {
        guard count > 0 else {
            window?.isHidden = true
            window = nil
            return
        }
        if window == nil {
            guard let scene = UIApplication.shared.connectedScenes
                .compactMap({ $0 as? UIWindowScene })
                .first(where: { $0.activationState == .foregroundActive }) ?? UIApplication.shared.connectedScenes
                .compactMap({ $0 as? UIWindowScene }).first else { return }
            let top = scene.windows.first?.safeAreaInsets.top ?? 44
            let size = CGSize(width: 150, height: 26)
            let w = UIWindow(windowScene: scene)
            w.frame = CGRect(x: scene.screen.bounds.width - size.width - 8, y: top, width: size.width, height: size.height)
            w.windowLevel = .alert + 1
            w.backgroundColor = .clear
            window = w
        }
        let host = UIHostingController(rootView: DebugFaultBadgeView(count: count))
        host.view.backgroundColor = .clear
        window?.rootViewController = host
        window?.isHidden = false
    }
}

private struct DebugFaultBadgeView: View {
    var count: Int

    var body: some View {
        Button { FaultInjection.removeAll() } label: {
            HStack(spacing: 4) {
                Image(systemName: "bolt.horizontal.circle.fill")
                Text(verbatim: "作り物 \(count) 件・止める")
            }
            .font(.caption2.weight(.semibold))
            .foregroundStyle(.white)
            .padding(.horizontal, 8)
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .background(Capsule().fill(Color.orange))
        }
        .buttonStyle(.plain)
        .accessibilityIdentifier("debugFaults.badge")
    }
}
#endif
