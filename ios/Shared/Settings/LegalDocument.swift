import SwiftUI

/// プライバシーポリシー・利用規約(#246・5.1.1(i))。アプリに同梱した `docs/legal/*.md` を表示し(オフラインでも読める)、
/// その画面から公開ページ(`publicURL`・App Store Connect にも同じ URL を登録)も開ける。
/// iOS と Mac で共有(#973)。Mac は同梱していないので公開ページを開く。
enum LegalDocument: String, Identifiable, CaseIterable {
    case privacyPolicy = "privacy-policy"
    case terms

    var id: String { rawValue }

    /// 公開ページ(LP)の URL。ここが唯一の設定箇所。
    var publicURL: URL? {
        switch self {
        case .privacyPolicy: URL(string: "https://tailor-it.app/privacy")
        case .terms: URL(string: "https://tailor-it.app/terms")
        }
    }

    /// お問い合わせの公開ページ(設定のサポート欄。審査用の版にも出す)。
    static let contactURL = URL(string: "https://tailor-it.app/contact")!

    var title: LocalizedStringKey {
        switch self {
        case .privacyPolicy: "プライバシーポリシー"
        case .terms: "利用規約"
        }
    }

    var systemImage: String {
        switch self {
        case .privacyPolicy: "hand.raised"
        case .terms: "doc.text"
        }
    }

    /// 同梱の Markdown(project.yml で docs/legal をバンドル)。
    var bundledText: String? {
        guard let url = Bundle.main.url(forResource: rawValue, withExtension: "md") else { return nil }
        return try? String(contentsOf: url, encoding: .utf8)
    }
}
