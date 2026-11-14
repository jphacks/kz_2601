// 審査用ビルド(APP_STORE_REVIEW)では外す: 審査用の版は AI が serverRules を作らない(#614)。
// iOS(Voice の修正チャット)と Mac(作る・直す会話のシート・#347)で共有する規則・保存・モデル。見た目の行はそれぞれのアプリに置く。
#if !APP_STORE_REVIEW
import Foundation
import Observation
import MetamonServices

/// 修正チャットを開いた時の「直しますか?」(#347・2026-11-09 決定)。
/// 続けて失敗している自動の動きがあれば、設定を開かなくてもチャットから直す依頼を入れられる。
enum AutomationFixOffer {
    /// 出すルール(1 本だけ)。運営が止めている・作成者がオフ・閉じた / 直した連続失敗は出さない。
    /// 複数あれば失敗の続いた回数が多い方、同じなら一覧の順。
    static func pick(_ automations: AppAutomations, handled: Set<String>) -> AutomationRule? {
        guard automations.stopped == nil else { return nil }
        let candidates = automations.rules.filter { rule in
            rule.suggestsFix && rule.stopped == nil && rule.enabled != false
                && !handled.contains(key(appId: automations.appId, rule: rule))
        }
        return candidates.enumerated()
            .max { a, b in
                a.element.consecutiveFailures != b.element.consecutiveFailures
                    ? a.element.consecutiveFailures < b.element.consecutiveFailures
                    : a.offset > b.offset
            }?.element
    }

    /// 連続失敗 1 回分の印。成功すると連続が 0 に戻り lastOkAt が進むので、次の連続では別の印になる。
    static func key(appId: String, rule: AutomationRule) -> String {
        let since = rule.lastOkAt.map { String(Int($0.timeIntervalSince1970)) } ?? "never"
        return "\(appId)|\(rule.id)|\(since)"
    }

    static func message(for rule: AutomationRule) -> String {
        L10n.tr("自動の動き「\(rule.description)」が続けて失敗しています。直しますか?")
    }
}

/// 閉じた / 直した印の保存(UserDefaults)。
struct AutomationFixOfferStore {
    nonisolated static let key = "metamon.automationFixOffer.handled.v1"
    /// 古い印から捨てる(アプリ × ルール × 連続の数は小さいが、際限なく増やさない)
    static let limit = 200

    let defaults: UserDefaults

    func handled() -> Set<String> { Set(defaults.stringArray(forKey: Self.key) ?? []) }

    func markHandled(_ k: String) {
        var list = defaults.stringArray(forKey: Self.key) ?? []
        guard !list.contains(k) else { return }
        list.append(k)
        defaults.set(Array(list.suffix(Self.limit)), forKey: Self.key)
    }
}

@MainActor
@Observable
final class AutomationFixOfferModel {
    let appId: String
    /// 出しているルール(nil = 出さない。読み込み中・失敗・作成者でない時も nil のまま)
    private(set) var rule: AutomationRule?
    @ObservationIgnored private let client: any AutomationsClient
    @ObservationIgnored private let store: AutomationFixOfferStore

    init(appId: String, client: any AutomationsClient, defaults: UserDefaults = .standard) {
        self.appId = appId
        self.client = client
        self.store = AutomationFixOfferStore(defaults: defaults)
    }

    /// 取れなければ(圏外・403 = 作成者でない など)何も出さない。
    func load() async {
        guard let a = try? await client.automations(appId: appId) else { return }
        rule = AutomationFixOffer.pick(a, handled: store.handled())
    }

    /// 「直す」: 印を付けて、入力欄に入れる依頼を返す(送るのは本人)。
    func fix() -> String? {
        guard let rule else { return nil }
        store.markHandled(AutomationFixOffer.key(appId: appId, rule: rule))
        self.rule = nil
        return AutomationsModel.fixRequest(for: rule)
    }

    func dismiss() {
        guard let rule else { return }
        store.markHandled(AutomationFixOffer.key(appId: appId, rule: rule))
        self.rule = nil
    }
}
#endif
