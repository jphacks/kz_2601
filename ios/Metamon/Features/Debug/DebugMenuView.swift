#if METAMON_STG
import SwiftUI
import MetamonServices

/// 開発用メニュー(#2079)。stg の版(Debug / Staging)だけ。開発者向けなので文言は日本語だけ(xcstrings に入れない)。
/// 先頭に今の環境、続けて `DebugMenuSection` の subclass(1 ファイル 1 つ)、最後に「全部元に戻す」。
struct DebugMenuView: View {
    @Environment(AppRuntime.self) private var rt
    @Environment(AuthSession.self) private var session
    @Environment(\.services) private var services
    @State private var state = DebugMenuState.shared
    @State private var copiedUID = false
    @State private var confirmReset = false

    var body: some View {
        List {
            environmentSection
            ForEach(DebugMenuRegistry.sections.map(SectionItem.init)) { item in
                Section {
                    item.type.makeBody()
                } header: {
                    Text(verbatim: item.type.title)
                } footer: {
                    if let footer = item.type.footer { Text(verbatim: footer) }
                }
            }
            if state.needsRelaunch {
                Section {
                    Button(role: .destructive) { exit(0) } label: {
                        Text(verbatim: "アプリを終了する")
                    }
                    .accessibilityIdentifier("debugMenu.quit")
                } footer: {
                    Text(verbatim: "起動し直しが要る項目を変えました。終了してから開き直すと効きます。")
                }
            }
            Section {
                Button(role: .destructive) { confirmReset = true } label: {
                    Text(verbatim: "全部元に戻す")
                }
                .accessibilityIdentifier("debugMenu.resetAll")
            } footer: {
                Text(verbatim: "このメニューで変えた値を全部消します。起動時の環境変数はそのまま効きます。")
            }
        }
        .navigationTitle(Text(verbatim: "開発用メニュー"))
        .navigationBarTitleDisplayMode(.inline)
        .confirmationDialog(Text(verbatim: "全部元に戻しますか？"), isPresented: $confirmReset, titleVisibility: .visible) {
            Button(role: .destructive) { resetAll() } label: { Text(verbatim: "元に戻す") }
            Button(role: .cancel) {} label: { Text(verbatim: "キャンセル") }
        }
        .accessibilityIdentifier("debugMenu")
    }

    private struct SectionItem: Identifiable {
        let type: DebugMenuSection.Type
        var id: ObjectIdentifier { ObjectIdentifier(type) }
    }

    private var environmentSection: some View {
        Section {
            row("構成", Self.configuration)
            row("版", "\(Self.info("CFBundleShortVersionString")) (\(Self.info("CFBundleVersion")))")
            row("ai", services.ai is MockAIClient ? "モック" : (BuildConfig.aiBaseURL?.absoluteString ?? "-"))
            row("appconfig", services.isLive ? (BuildConfig.appConfigBaseURL?.absoluteString ?? "-") : "インメモリ")
            row("Firebase", Self.info("MetamonFirebaseEnv"))
            HStack {
                row("uid", session.user?.uid ?? "未ログイン")
                if let uid = session.user?.uid {
                    Button {
                        UIPasteboard.general.string = uid
                        copiedUID = true
                    } label: {
                        Image(systemName: copiedUID ? "checkmark" : "doc.on.doc")
                    }
                    .buttonStyle(.borderless)
                    .accessibilityLabel(Text(verbatim: "uid をコピー"))
                    .accessibilityIdentifier("debugMenu.copyUID")
                }
            }
            row("匿名", session.user.map { $0.isAnonymous ? "はい" : "いいえ" } ?? "-")
            row("appId", rt.document.appId)
        } header: {
            Text(verbatim: "今の環境")
        }
        .accessibilityIdentifier("debugMenu.environment")
    }

    private func row(_ label: String, _ value: String) -> some View {
        LabeledContent {
            Text(verbatim: value)
                .font(.footnote.monospaced())
                .lineLimit(2)
                .truncationMode(.middle)
                .textSelection(.enabled)
        } label: {
            Text(verbatim: label)
        }
    }

    private func resetAll() {
        DebugKnobs.resetAll()
        for section in DebugMenuRegistry.sections { section.reset() }
        state.markNeedsRelaunch() // 起動時に読んだつまみは開き直すまで残る
    }

    static var configuration: String {
        #if DEBUG
        "Debug"
        #else
        "Staging"
        #endif
    }

    private static func info(_ key: String) -> String {
        (Bundle.main.object(forInfoDictionaryKey: key) as? String).flatMap { $0.isEmpty || $0.hasPrefix("$(") ? nil : $0 } ?? "-"
    }
}

/// 設定の末尾の入口(`SettingsSheet`)
struct DebugMenuEntrySection: View {
    var body: some View {
        Section {
            NavigationLink {
                DebugMenuView()
            } label: {
                Label { Text(verbatim: "開発用メニュー") } icon: { Image(systemName: "wrench.and.screwdriver") }
            }
            .accessibilityIdentifier("settings.debugMenu")
        } footer: {
            Text(verbatim: "stg の版だけに出ます。")
        }
    }
}
#endif
