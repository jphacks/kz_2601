#if !APP_STORE_REVIEW // 審査用の版は OAuth の provider と App 内課金を外す(決定 6・#450)
import Foundation
import MetamonServices

// iOS と Mac で共有する外の API の断りの判定(#2001・Mac は #3078)。画面(シート・トースト)は各アプリ側。

/// 外の API の断り(`ExtDenial`・#2001)の「次にすること」。判定は純粋関数(テストする)。
enum ExtDenialRoute: Equatable {
    /// つないでいない → つなぐ案内のシート
    case connect(provider: String)
    /// Pro が要る → 課金画面
    case paywall([PaidFeature])
    /// コイン → `CoinGate`(お店 / コインの設定)。引き受けられなければ説明のトースト
    case coins(CoinDenial)
    /// シートを出さず 1 文だけ
    case toast(String)

    static func of(_ denial: ExtDenial, isPro: Bool) -> ExtDenialRoute {
        switch denial {
        case .notConnected(let provider?):
            return .connect(provider: provider)
        case .notConnected(nil):
            return .toast(ExtDenialCopy.notConnectedUnknown)
        case .proRequired(let pro):
            if pro.creatorNeedsPro { return .toast(pro.extMessage) } // 参加者は払っても直らない
            if isPro { return .toast(ExtDenialCopy.proNotSynced) } // 端末は Pro なのにサーバーが知らない。課金画面を繰り返さない
            return .paywall(pro.features.isEmpty ? [ExtDenialCopy.externalServicesFeature] : pro.features)
        case .coins(let coins):
            return .coins(coins)
        case .unavailable:
            return .toast(ExtDenialCopy.unavailable)
        case .busy:
            return .toast(ExtDenialCopy.busy)
        }
    }
}

enum ExtDenialCopy {
    static func connectTitle(_ name: String) -> String { L10n.tr("\(name) とつないでください") }
    static func connectBody(_ name: String) -> String {
        L10n.tr("この部品は \(name) のデータを使います。つなぐと続けられます。つないだサービスは設定からいつでも切れます。")
    }
    static var connectUnavailable: String { L10n.tr("このサービスはいまつなげません。しばらくしてからお試しください。") }
    static var notConnectedUnknown: String { L10n.tr("外のサービスとつながっていません。設定の「つないだサービス」からつないでください。") }
    static var proNotSynced: String { L10n.tr("購入の確認がまだサーバーに届いていません。少し待ってから、もう一度お試しください。") }
    static var unavailable: String { L10n.tr("外のサービスはいま使えません。しばらくしてからお試しください。") }
    static var busy: String { L10n.tr("外のサービスが混み合っています。少し待ってから、もう一度お試しください。") }
    /// 外の API が Pro 限定である説明(設定の「つないだサービス」と課金画面で同じもの)
    static var externalServicesFeature: PaidFeature {
        PaidFeature(id: "externalServices", title: L10n.tr("つないだサービス"),
                    reason: L10n.tr("外のサービスを呼ぶたびにサーバーの費用がかかるため、Pro の機能です。"))
    }
    static var closeNote: String { L10n.tr("閉じると、この部品は外のサービスを呼びません。") }
}

/// つなぐ案内の「つなぐ」を押せるか。一覧を読み終わるまでは押せない(読む前に押すと今つなげない provider でも始めてしまう)。
enum ExtConnectAvailability: Equatable {
    case loading, available, unavailable

    /// `providersKnown`: 一覧に `providers` があったか。あるのにこの provider が無ければ今つなげない
    /// (`ConnectedServicesState.rows` と同じく enabled=false 扱い)。無い古いサーバーだけつなげる扱い。
    static func of(_ state: ConnectedServicesState, provider: String, providersKnown: Bool) -> ExtConnectAvailability {
        switch state {
        case .loading: .loading
        case .hidden: .unavailable // サーバーが外の API を止めている
        case .failed: .available // 一覧が読めないだけ。つなぐ流れ側でもう一度確かめる
        case .loaded(let rows):
            rows.first { $0.provider == provider }.map { $0.enabled ? .available : .unavailable }
                ?? (providersKnown ? .unavailable : .available)
        }
    }

    /// つなぐ案内を閉じるか。読み終わった一覧で「つないでいない」→「つないだ」に変わった時だけ。
    /// 最初の読み込み(.loading → つないだ)は閉じない: サーバーは ext_not_connected と言っている(トークン切れ等)。
    static func didConnect(from old: ConnectedServicesState, to new: ConnectedServicesState, provider: String) -> Bool {
        guard case .loaded(let after) = new, after.first(where: { $0.provider == provider })?.status == .connected else { return false }
        // 前が読めていない時は、つないだ結果か最初からつながっていたか区別できないので閉じない(閉じるボタンはある)
        guard case .loaded(let before) = old else { return false }
        return before.first { $0.provider == provider }?.status != .connected
    }
}
#endif
