import SwiftUI
import MetamonServices

/// アシスティブアクセス(#1997)の中で出す・出さないの決まり。判定もここ 1 か所に集める。
///
/// アシスティブアクセスでは作る機能(音声で生成・直す)・AI ボタン・設定・課金・起動時の案内を出さず、
/// 最初の画面を生成アプリの一覧にする。開いた生成アプリは主な動作の大きなボタン(#1998・選べなければ今の画面)。
struct AssistiveAccessPolicy: Equatable, Sendable {
    var isActive: Bool

    static let standard = AssistiveAccessPolicy(isActive: false)

    /// アシスティブアクセスで隠すもの。
    enum Feature: CaseIterable, Sendable {
        /// マイクの FAB・ナビバーのマイク(作る・直す入口)
        case aiButton
        /// 話しかける画面(生成・改変)
        case voiceSheet
        /// 変身一覧のシート(新しいアプリ・設定・共有の入口がある)
        case appSwitcher
        /// 課金の画面
        case paywall
        /// 起動時の AI 送信の同意・チュートリアル・β の案内・引き継ぎの案内
        case launchPrompts
        /// 生成アプリの AI 部品から出す AI 送信の同意(`requireAIConsent`)。代わりに `unavailableMessage`
        case aiConsent
        /// シェアリンクから開く追加・参加の確認と「探す」(`SharingHooks`)。届いたリンクは開かず `unavailableMessage`
        case shareLinks
    }

    /// アシスティブアクセスで隠すもの(今は全部)。
    static let hiddenFeatures: Set<Feature> = [.aiButton, .voiceSheet, .appSwitcher, .paywall, .launchPrompts, .aiConsent, .shareLinks]

    func shows(_ feature: Feature) -> Bool { !isActive || !Self.hiddenFeatures.contains(feature) }

    /// 隠した機能を呼ばれた時の一言。
    static var unavailableMessage: String { L10n.tr("この画面では使えません") }

    /// 根に出すもの。
    enum Home: Equatable, Sendable {
        /// いつもの画面(アシスティブアクセスでない)
        case standard
        /// 生成アプリの一覧
        case appList
        /// 一覧から開いた生成アプリ(主な動作のボタン・選べなければ今の画面: `AssistiveActions.hasCandidates`)
        case openedApp
    }

    /// `openedAppId` = 一覧から開いたアプリ。今のアプリと一致する時だけ開いた画面を出す(切り替えに失敗したら一覧のまま)。
    func home(openedAppId: String?, currentAppId: String) -> Home {
        guard isActive else { return .standard }
        guard let openedAppId, openedAppId == currentAppId else { return .appList }
        return .openedApp
    }

    /// 一覧に出すアプリ: archived と未生成(初期画面のまま)を除く。並びは `visibleApps` のまま。
    static func listedApps(_ apps: [AppSummary]) -> [AppSummary] {
        apps.filter { !$0.isArchived && !$0.isBlank }
    }

    /// システムの判定(iOS 18 以降)と、DEBUG の起動引数 `METAMON_ASSISTIVE_ACCESS=1`(シミュレータで見た目を確かめる用)。
    static func detect(systemEnabled: Bool, debugOverride: String?) -> AssistiveAccessPolicy {
        AssistiveAccessPolicy(isActive: systemEnabled || debugOverride == "1")
    }
}

private struct AssistiveAccessPolicyKey: EnvironmentKey {
    static let defaultValue = AssistiveAccessPolicy.standard
}

extension EnvironmentValues {
    /// アシスティブアクセスの決まり。`AssistiveAccessReader` が根で入れる。
    var assistiveAccess: AssistiveAccessPolicy {
        get { self[AssistiveAccessPolicyKey.self] }
        set { self[AssistiveAccessPolicyKey.self] = newValue }
    }
}

/// アシスティブアクセスの中かを読む唯一の口。`MetamonApp` の根に 1 つだけ付ける。
/// `accessibilityAssistiveAccessEnabled` は iOS 18 から(iOS 17 では常に外)。
struct AssistiveAccessReader: ViewModifier {
    func body(content: Content) -> some View {
        if #available(iOS 18.0, *) {
            content.modifier(SystemReader())
        } else {
            content.environment(\.assistiveAccess, .detect(systemEnabled: false, debugOverride: Self.debugOverride))
        }
    }

    static var debugOverride: String? {
        #if DEBUG
        ProcessInfo.processInfo.environment["METAMON_ASSISTIVE_ACCESS"]
        #else
        nil
        #endif
    }

    @available(iOS 18.0, *)
    private struct SystemReader: ViewModifier {
        @Environment(\.accessibilityAssistiveAccessEnabled) private var enabled

        func body(content: Content) -> some View {
            content.environment(\.assistiveAccess, .detect(systemEnabled: enabled, debugOverride: AssistiveAccessReader.debugOverride))
        }
    }
}

extension AppRuntime {
    /// `feature` を出せるか。アシスティブアクセスで隠している時は `unavailableMessage` を出して false。
    func showsOrToast(_ feature: AssistiveAccessPolicy.Feature) -> Bool {
        guard !assistiveAccess.shows(feature) else { return true }
        toast = AssistiveAccessPolicy.unavailableMessage
        return false
    }
}

/// アシスティブアクセスでない時だけ `modifier` を付ける(`feature` を出さない時は付けない)。
struct UnlessAssistiveAccess<M: ViewModifier>: ViewModifier {
    let feature: AssistiveAccessPolicy.Feature
    let modifier: M
    @Environment(\.assistiveAccess) private var assistiveAccess

    func body(content: Content) -> some View {
        if assistiveAccess.shows(feature) {
            content.modifier(modifier)
        } else {
            content
        }
    }
}

extension View {
    func unlessAssistiveAccess<M: ViewModifier>(_ feature: AssistiveAccessPolicy.Feature, _ modifier: M) -> some View {
        self.modifier(UnlessAssistiveAccess(feature: feature, modifier: modifier))
    }
}
