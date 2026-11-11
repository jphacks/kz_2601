import SwiftUI

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
