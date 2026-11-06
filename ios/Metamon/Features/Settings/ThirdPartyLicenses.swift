import SwiftUI

/// 設定の「ライセンス」(#306)。このアプリ自身の MIT の表示と、同梱する依存のライセンス。
/// 中身は `Resources/ThirdPartyLicenses.json`(`ios/scripts/third-party-licenses.py` が Package.resolved から生成)。
/// 審査用の版にも出す(ライセンスの表示は外さない)。
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

/// 設定の行。
struct LicensesLink: View {
    var body: some View {
        NavigationLink {
            LicensesView()
        } label: {
            Label("ライセンス", systemImage: "doc.plaintext")
        }
        .font(.body)
        .accessibilityIdentifier("settings.licenses")
    }
}

struct LicensesView: View {
    private let entries = ThirdPartyLicenses.load()

    var body: some View {
        List {
            if let entries {
                let app = entries.filter { $0.kind == .app }
                let deps = entries.filter { $0.kind == .dependency }
                Section("このアプリ") {
                    ForEach(app) { row($0) }
                }
                Section {
                    ForEach(deps) { row($0) }
                } header: {
                    Text("オープンソースソフトウェア")
                } footer: {
                    Text("このアプリは上のオープンソースソフトウェアを使っています。")
                }
            } else {
                Text("ライセンスを読み込めませんでした").foregroundStyle(.secondary)
            }
        }
        .navigationTitle("ライセンス")
        .navigationBarTitleDisplayMode(.inline)
    }

    private func row(_ entry: LicenseEntry) -> some View {
        NavigationLink {
            LicenseDetailView(entry: entry)
        } label: {
            VStack(alignment: .leading, spacing: 2) {
                Text(verbatim: entry.name)
                Text(verbatim: [entry.version, entry.license].compactMap { $0 }.joined(separator: " · "))
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }
            .accessibilityElement(children: .combine)
        }
        .accessibilityIdentifier("licenses.\(entry.id)")
    }
}

struct LicenseDetailView: View {
    let entry: LicenseEntry

    var body: some View {
        ScrollView {
            LazyVStack(alignment: .leading, spacing: Theme.Spacing.sm) {
                VStack(alignment: .leading, spacing: Theme.Spacing.xs) {
                    Text(verbatim: entry.name).font(.title3.weight(.semibold))
                        .accessibilityAddTraits(.isHeader)
                    if let version = entry.version { LabeledContent("版") { Text(verbatim: version) } }
                    LabeledContent("ライセンス") { Text(verbatim: entry.license) }
                    ForEach(entry.copyright, id: \.self) { Text(verbatim: $0).font(.footnote) }
                    if let s = entry.url, let url = URL(string: s) {
                        Link(destination: url) { Label("ソースコード", systemImage: "safari") }
                            .font(.callout)
                    }
                }
                .padding(.bottom, Theme.Spacing.sm)
                paragraphs(entry.text)
                ForEach(entry.notices, id: \.self) { notice in
                    Text(verbatim: notice.title)
                        .font(.headline)
                        .padding(.top, Theme.Spacing.md)
                        .accessibilityAddTraits(.isHeader)
                    paragraphs(notice.text)
                }
            }
            .padding(Theme.Spacing.lg)
            .frame(maxWidth: .infinity, alignment: .leading)
            .textSelection(.enabled)
        }
        .navigationTitle(Text(verbatim: entry.name))
        .navigationBarTitleDisplayMode(.inline)
    }

    private func paragraphs(_ text: String) -> some View {
        ForEach(Array(ThirdPartyLicenses.paragraphs(text).enumerated()), id: \.offset) { _, p in
            Text(verbatim: p)
                .font(.footnote)
                .frame(maxWidth: .infinity, alignment: .leading)
        }
    }
}
