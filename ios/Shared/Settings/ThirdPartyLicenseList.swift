import Foundation

/// 設定の「ライセンス」(#306)。このアプリ自身の MIT の表示と、同梱する依存のライセンス。
/// 中身は `Resources/ThirdPartyLicenses.json`(`ios/scripts/third-party-licenses.py` が Package.resolved から生成)。
/// 審査用の版にも出す(ライセンスの表示は外さない)。一覧の読み込みは iOS と Mac で共有(#973)、画面は各 OS の側。
struct LicenseEntry: Decodable, Identifiable, Hashable {
    enum Kind: String, Decodable { case app, dependency }
    struct Notice: Decodable, Hashable {
        let title: String
        let text: String
    }

    let id: String
    let kind: Kind
    let name: String
    let version: String?
    let url: String?
    let license: String
    let copyright: [String]
    let text: String
    let notices: [Notice]
}

enum ThirdPartyLicenses {
    static let resourceName = "ThirdPartyLicenses"

    /// 同梱の一覧。読めなければ nil。
    static func load(bundle: Bundle = .main) -> [LicenseEntry]? {
        guard let url = bundle.url(forResource: resourceName, withExtension: "json"),
              let data = try? Data(contentsOf: url) else { return nil }
        struct File: Decodable { let entries: [LicenseEntry] }
        return try? JSONDecoder().decode(File.self, from: data).entries
    }

    /// 本文を段落に分ける(空行区切り・段落内の改行と字下げは詰める)。大きい文字で折り返し、VoiceOver で段落ごとに読めるように。
    static func paragraphs(_ text: String) -> [String] {
        text.components(separatedBy: "\n")
            .split(whereSeparator: { $0.trimmingCharacters(in: .whitespaces).isEmpty })
            .map { $0.map { $0.trimmingCharacters(in: .whitespaces) }.joined(separator: " ") }
    }
}
