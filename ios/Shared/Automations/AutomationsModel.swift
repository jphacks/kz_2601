// 審査用ビルド(APP_STORE_REVIEW)では外す: 審査用の版は AI が serverRules を作らない(#614)。
#if !APP_STORE_REVIEW
import Foundation
import Observation
import MetamonServices

/// 自動の動き(#347)の一覧とオン / オフ。iOS(`AutomationsView`)と Mac(#979)で共有する。
@MainActor
@Observable
final class AutomationsModel {
    enum Phase: Equatable {
        case loading
        case loaded(AppAutomations)
        /// 作成者ではない(403)
        case notCreator
        case failed(String)
    }

    let appId: String
    private let client: any AutomationsClient
    private(set) var phase: Phase = .loading
    /// 切り替えを送っている最中のルール(二重に押させない)
    private(set) var pending: Set<String> = []
    /// 切り替えに失敗した時の案内(アラート)
    var toggleError: String?

    init(appId: String, client: any AutomationsClient) {
        self.appId = appId
        self.client = client
    }

    func load() async {
        do {
            phase = .loaded(try await client.automations(appId: appId))
        } catch let e as APIError where e.status == 403 {
            phase = .notCreator
        } catch {
            if case .loaded = phase { return } // 前の結果を残す
            phase = .failed(error.localizedDescription)
        }
    }

    /// ルールのオン / オフ。先に見た目を変え(楽観的)、失敗したら戻す。他で変わっていた(412 など)時は取り直す。
    func setEnabled(_ enabled: Bool, ruleId: String) async {
        guard case .loaded(var a) = phase, let i = a.rules.firstIndex(where: { $0.id == ruleId }),
              let before = a.rules[i].enabled, before != enabled, !pending.contains(ruleId) else { return }
        a.rules[i].enabled = enabled
        phase = .loaded(a)
        pending.insert(ruleId)
        defer { pending.remove(ruleId) }
        do {
            try await client.setRule(ruleId, enabled: enabled, appId: appId)
        } catch {
            if case .loaded(var cur) = phase, let j = cur.rules.firstIndex(where: { $0.id == ruleId }) {
                cur.rules[j].enabled = before
                phase = .loaded(cur)
            }
            if AutomationToggle.isConflict(error) {
                toggleError = L10n.tr("ほかの変更と重なったため切り替えませんでした。最新の状態を読み込み直しました。")
                await load()
            } else {
                toggleError = L10n.tr("切り替えられませんでした。時間をおいてもう一度お試しください。")
            }
        }
    }

    /// 「直しますか?」で入力欄に入れる依頼(送るのは本人)。
    static func fixRequest(for rule: AutomationRule) -> String {
        L10n.tr("自動の動き「\(rule.description)」が続けてうまくいっていません。うまく動くように直して")
    }
}
#endif
