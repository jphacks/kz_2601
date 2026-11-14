import SwiftUI

/// 設定の「ライセンス」(#306 の Mac 版・#973)。一覧は iOS と同じ `ThirdPartyLicenses.json`。左に一覧、右に本文。
struct MacLicensesView: View {
    @Environment(\.dismiss) private var dismiss
    private let entries = ThirdPartyLicenses.load()
    @State private var selection: LicenseEntry?

    var body: some View {
        NavigationSplitView {
            if let entries {
                List(selection: $selection) {
                    Section("このアプリ") {
                        ForEach(entries.filter { $0.kind == .app }) { row($0) }
                    }
                    Section {
                        ForEach(entries.filter { $0.kind == .dependency }) { row($0) }
                    } header: {
                        Text("オープンソースソフトウェア")
                    } footer: {
                        Text("このアプリは上のオープンソースソフトウェアを使っています。")
                    }
                }
                .navigationSplitViewColumnWidth(min: 200, ideal: 240)
            } else {
                Text("ライセンスを読み込めませんでした").foregroundStyle(.secondary)
            }
        } detail: {
            if let selection {
                MacLicenseDetailView(entry: selection)
            } else {
                Text("ライセンス").foregroundStyle(.secondary)
            }
        }
        .navigationTitle("ライセンス")
        .frame(minWidth: 680, minHeight: 460)
        .toolbar {
            ToolbarItem(placement: .cancellationAction) {
                Button("閉じる") { dismiss() }
            }
        }
        .onAppear { selection = selection ?? entries?.first }
    }

    private func row(_ entry: LicenseEntry) -> some View {
        VStack(alignment: .leading, spacing: MacTheme.Spacing.rowLine) {
            Text(verbatim: entry.name)
            Text(verbatim: [entry.version, entry.license].compactMap { $0 }.joined(separator: " · "))
                .font(.subheadline)
                .foregroundStyle(.secondary)
        }
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("licenses.\(entry.id)")
        .tag(entry)
    }
}

private struct MacLicenseDetailView: View {
    let entry: LicenseEntry

    var body: some View {
        ScrollView {
            LazyVStack(alignment: .leading, spacing: Theme.Spacing.sm) {
                VStack(alignment: .leading, spacing: Theme.Spacing.xs) {
                    Text(verbatim: entry.name).font(.title3.weight(.semibold))
                        .accessibilityAddTraits(.isHeader)
                    if let version = entry.version { LabeledContent("版") { Text(verbatim: version) } }
                    LabeledContent("ライセンス") { Text(verbatim: entry.license) }
                    ForEach(entry.copyright, id: \.self) { Text(verbatim: $0) }
                    if let s = entry.url, let url = URL(string: s) {
                        Link(destination: url) { Label("ソースコード", systemImage: "safari") }
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
            .padding(MacTheme.Spacing.window)
            .frame(maxWidth: .infinity, alignment: .leading)
            .textSelection(.enabled)
        }
    }

    private func paragraphs(_ text: String) -> some View {
        ForEach(Array(ThirdPartyLicenses.paragraphs(text).enumerated()), id: \.offset) { _, p in
            Text(verbatim: p)
                .frame(maxWidth: .infinity, alignment: .leading)
        }
    }
}
