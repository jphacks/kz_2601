// 審査用ビルド(APP_STORE_REVIEW・#245)ではリリース申請を外す。
#if !APP_STORE_REVIEW
import Foundation
import MetamonServices

// リリース申請(#76)の文言と入力の決まり。iOS の `ReleaseRequestSheet` と Mac の `MacReleaseRequestSheet`(#2249)で共有する。

extension ReleaseStatus {
    var label: String {
        switch self {
        case .received: return L10n.tr("受付")
        case .reviewing: return L10n.tr("検討中")
        case .planned: return L10n.tr("リリース予定")
        case .declined: return L10n.tr("見送り")
        }
    }

    var symbol: String {
        switch self {
        case .received: return "tray.and.arrow.down"
        case .reviewing: return "magnifyingglass"
        case .planned: return "shippingbox"
        case .declined: return "xmark.circle"
        }
    }

    /// バッジの読み上げ(「リリース申請: 検討中」)
    var accessibilityLabel: String { L10n.tr("リリース申請: \(label)") }
}

/// 申請のシートの入力: 用途の一言(必須・200 字)と連絡先(任意・100 字)。
enum ReleaseRequestForm {
    static let maxPurpose = 200
    static let maxContact = 100

    /// 上限で切る(入力のたびに呼ぶ)
    static func clipped(_ text: String, max: Int) -> String {
        text.count > max ? String(text.prefix(max)) : text
    }

    /// 送れるか: 用途が空白だけでない・送っている最中でない
    static func canSend(purpose: String, busy: Bool) -> Bool {
        !purpose.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty && !busy
    }

    /// 送る値: 用途は前後の空白を外し、連絡先は空なら nil
    static func values(purpose: String, contact: String) -> (purpose: String, contact: String?) {
        let c = contact.trimmingCharacters(in: .whitespacesAndNewlines)
        return (purpose.trimmingCharacters(in: .whitespacesAndNewlines), c.isEmpty ? nil : c)
    }

    /// フォームの代わりに状況を出す申請(申請中 = 見送り以外)。見送りなら出し直せる
    static func open(_ request: ReleaseRequest?) -> ReleaseRequest? {
        guard let request, request.kind != .declined else { return nil }
        return request
    }
}
#endif
