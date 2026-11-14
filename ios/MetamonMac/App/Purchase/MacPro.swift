// 審査用ビルド(APP_STORE_REVIEW・#450 G1)は App 内課金を持たない(iOS と同じ)。
#if !APP_STORE_REVIEW
import AppKit
import Observation
import MetamonServices

/// 課金画面を開く理由(iOS の `PaywallReason` と同じ分け方)。見出しを変える。
enum MacPaywallReason: Equatable {
    /// 設定の「Pro にする…」
    case settings
    /// プランの上限(作れるアプリの数など)。Pro で上限が増える
    case planLimit
    /// オンラインの AI など、Pro が要る機能(features が空なら「みんなで共有するデータ」)
    case paidFeature([PaidFeature])
}

/// 課金画面の表示要求。
struct MacPaywallRequest: Identifiable, Equatable {
    let id = UUID()
    var reason: MacPaywallReason
    /// 閉じた(買わなかった)時にどうなるかの一言
    var closeNote: String?
    /// 出すウインドウ(`MacPro.windowKey`)。頼んだ時の前面のウインドウ。nil なら課金画面を出せるウインドウがまだ無い
    var window: Int?
}

/// Mac の Tailor It Pro(iOS の `ProGate` の Mac 版)。商品は iOS と同じ `ProProducts`、Bundle ID も同じなので、
/// どちらで買っても同じ Apple ID なら両方で Pro になる(StoreKit 2 の `Transaction.currentEntitlements`)。
/// 無料作成の回数(端末で数える)は Mac にまだ無いので持たない。
@MainActor
@Observable
final class MacPro {
    static let shared = MacPro()

    let store: ProEntitlementStore
    /// 中央のアプリの部品(オンラインの AI)から出す課金画面。メインのウインドウが出す(`MacPaywallPresenter`)
    private(set) var paywall: MacPaywallRequest?
    /// 課金画面を出せるメインのウインドウ(`MacPaywallPresenter` が入れる)。付いた順
    @ObservationIgnored private(set) var presenterWindows: [Int] = []
    /// 前面から順のウインドウの印(テストで差し替える)
    @ObservationIgnored var frontWindows: @MainActor () -> [Int] = { MacPro.frontWindowNumbers() }

    init(store: ProEntitlementStore? = nil) {
        self.store = store ?? ProEntitlementStore(mode: Self.storeMode)
    }

    var isPro: Bool { store.isPro }

    /// 起動時に 1 回(2 回目以降は権利を読み直すだけ)
    func start() { store.start() }

    /// 課金画面を出す。Pro なら出さない(設定から開く時は呼ばずに直接出す)
    func requestPro(_ reason: MacPaywallReason, closeNote: String? = nil) {
        guard !isPro else { return }
        paywall = MacPaywallRequest(reason: reason, closeNote: closeNote,
                                    window: Self.target(front: frontWindows(), presenters: presenterWindows))
    }

    func dismissPaywall() { paywall = nil }

    /// 課金画面を出すウインドウ: 前面(キー)のウインドウ。それが設定などで出せなければ、出せる中で一番前・無ければ最初に付いたもの
    static func target(front: [Int], presenters: [Int]) -> Int? {
        front.first(where: presenters.contains) ?? presenters.first
    }

    /// キー → メイン → 前から順のウインドウ。シートのウインドウは親のウインドウに読み替える
    static func frontWindowNumbers() -> [Int] {
        func root(_ window: NSWindow) -> NSWindow {
            var window = window
            while let parent = window.sheetParent { window = parent }
            return window
        }
        let windows = [NSApp.keyWindow, NSApp.mainWindow].compactMap { $0 } + NSApp.orderedWindows
        return windows.map { windowKey(root($0)) }
    }

    /// ウインドウの印。番号(`windowNumber`)は画面に出るまで決まらないので、ウインドウそのものの識別子を使う
    static func windowKey(_ window: NSWindow) -> Int { ObjectIdentifier(window).hashValue }

    /// メインのウインドウが課金画面を出せるようになった。宛先の無い要求があればここで出す
    func registerPresenter(window: Int) {
        guard !presenterWindows.contains(window) else { return }
        presenterWindows.append(window)
        if paywall != nil, paywall?.window == nil { paywall?.window = window }
    }

    /// ウインドウが閉じた。そこに出していた課金画面は閉じたものとする(ほかのウインドウには移さない)
    func unregisterPresenter(window: Int) {
        presenterWindows.removeAll { $0 == window }
        if paywall?.window == window { paywall = nil }
    }

    /// このウインドウに出す要求か
    func paywall(for window: Int?) -> MacPaywallRequest? {
        guard let window, let paywall, paywall.window == window else { return nil }
        return paywall
    }

    /// サブスクリプションの管理(解約・プランの確認)は Apple のアカウントの画面で行う。
    /// iOS の `manageSubscriptionsSheet` は macOS に無いので、App Store のページを開く
    static let manageSubscriptionsURL = URL(string: "https://apps.apple.com/account/subscriptions")!

    static func openManageSubscriptions() {
        NSWorkspace.shared.open(manageSubscriptionsURL)
    }

    /// iOS の `ProGate.storeMode` と同じ: Debug は `METAMON_MOCK_STORE`(1 = モック・0 = StoreKit)、
    /// 無ければ StoreKit で商品が取れない時にモックへ(手元の Run に StoreKit の構成ファイルが無いため)
    private static var storeMode: ProEntitlementStore.Mode {
        #if DEBUG
        switch ProcessInfo.processInfo.environment["METAMON_MOCK_STORE"] {
        case "1": return .mock
        case "0": return .storeKit
        default: return .storeKitOrMock
        }
        #else
        return .storeKit
        #endif
    }
}
#endif
