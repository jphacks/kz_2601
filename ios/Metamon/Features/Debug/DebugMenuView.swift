// 開発用メニュー(#2079)。`METAMON_STG` を立てた構成にしか入らない(DebugMenuPolicy.swift の頭の説明)。
#if METAMON_STG
import SwiftUI
import MetamonServices

/// 設定の末尾から開く開発用メニュー。文言は開発者向けなので日本語だけ(`Text(verbatim:)` で xcstrings に入れない)。
///
/// 中身の順番:
/// 1. 今の環境(構成・版 / ビルド・接続先・Firebase・uid・匿名か・appId)
/// 2. つまみ(`DebugKnobCatalog.all`。この Issue では空で、子の Issue #2080 以降が足す)
/// 3. 開発用の操作(設定に散らばっていた DEBUG だけの項目をここへ集めた)
/// 4. 全部元に戻す
struct DebugMenuView: View {
    @Environment(AuthSession.self) private var session
    @Environment(\.services) private var services
    @State private var copied: String?
    /// 「起動し直しが要る」つまみを変えたか(変えたら「アプリを終了する」を出す)
    @State private var changedRelaunchKnob = false
    @State private var confirmReset = false
    @State private var message: String?
    #if !APP_STORE_REVIEW
    /// 外の API の断りの案内(#2001)。「試験用のサービスを呼ぶ」が入れる
    @State private var denial: ExtDenial?
    #endif

    var body: some View {
        List {
            environmentSection
            knobsSection
            actionsSection
            resetSection
        }
        .navigationTitle(Text(verbatim: "開発用メニュー"))
        .navigationBarTitleDisplayMode(.inline)
        .accessibilityIdentifier("debugMenu")
        #if !APP_STORE_REVIEW
        .extDenialSheets($denial) { message = $0 }
        #endif
        .confirmationDialog(Text(verbatim: "開発用のつまみを全部元に戻しますか？"),
                            isPresented: $confirmReset, titleVisibility: .visible) {
            Button(role: .destructive) {
                DebugKnobs.resetAll()
                changedRelaunchKnob = true
                message = "元に戻しました。起動し直すと効きます。"
            } label: {
                Text(verbatim: "全部元に戻す")
            }
            .accessibilityIdentifier("debugMenu.reset.confirm")
            Button(role: .cancel) {} label: { Text(verbatim: "キャンセル") }
        } message: {
            Text(verbatim: "メニューで保存した値(\(DebugKnobs.defaultsPrefix)*)だけを消します。アプリの設定・記録は消しません。")
        }
    }

    // MARK: - 今の環境

    private var rows: [DebugEnvironmentRow] {
        let bundle = Bundle.main
        return DebugEnvironment.rows(
            version: bundle.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String,
            build: bundle.object(forInfoDictionaryKey: "CFBundleVersion") as? String,
            aiURL: services.ai is MockAIClient ? nil : BuildConfig.aiBaseURL,
            appConfigURL: BuildConfig.appConfigBaseURL,
            isLiveAppConfig: services.isLive,
            firebaseEnv: DebugEnvironment.firebaseEnv(),
            uid: session.user?.uid,
            isAnonymous: session.user?.isAnonymous,
            appId: DeviceAppID.current)
    }

    private var environmentSection: some View {
        let rows = self.rows
        return Section {
            ForEach(rows) { row in
                if row.copyable {
                    Button { copy(row) } label: { environmentRow(row) }
                        .accessibilityIdentifier("debugMenu.env.\(row.label)")
                } else {
                    environmentRow(row)
                }
            }
            Button {
                UIPasteboard.general.string = DebugEnvironment.clipboardText(rows)
                copied = "すべて"
            } label: {
                Label { Text(verbatim: copied == "すべて" ? "コピーしました" : "全部コピー") }
                icon: { Image(systemName: copied == "すべて" ? "checkmark" : "doc.on.doc") }
            }
            .accessibilityIdentifier("debugMenu.env.copyAll")
        } header: {
            Text(verbatim: "今の環境")
        } footer: {
            Text(verbatim: "uid と appId は押すとコピーできます。")
        }
    }

    private func environmentRow(_ row: DebugEnvironmentRow) -> some View {
        LabeledContent {
            Text(verbatim: copied == row.label ? "コピーしました" : row.value)
                .font(.footnote.monospaced())
                .lineLimit(1)
                .truncationMode(.middle)
                .textSelection(.enabled)
        } label: {
            Text(verbatim: row.label)
        }
    }

    private func copy(_ row: DebugEnvironmentRow) {
        UIPasteboard.general.string = row.value
        copied = row.label
    }

    // MARK: - つまみ

    private var knobsSection: some View {
        Section {
            if DebugKnobCatalog.all.isEmpty {
                Text(verbatim: "まだありません(子の Issue #2080 以降で足します)")
                    .foregroundStyle(.secondary)
            } else {
                ForEach(DebugKnobCatalog.all) { knob in
                    DebugKnobRow(knob: knob) { changed in
                        if changed.effect == .needsRelaunch { changedRelaunchKnob = true }
                    }
                }
            }
        } header: {
            Text(verbatim: "つまみ")
        } footer: {
            Text(verbatim: "起動時に環境変数(\u{0022}METAMON_…\u{0022})を渡していると、そちらが勝ちます(行に「環境変数で固定中」と出ます)。")
        }
    }

    // MARK: - 開発用の操作(設定から移したもの)

    private var actionsSection: some View {
        #if !APP_STORE_REVIEW
        @Bindable var gate = ProGate.shared
        #endif
        return Section {
            #if !APP_STORE_REVIEW
            // 設定の「Tailor It Pro」から移設(#2079)
            Toggle(isOn: $gate.limitDisabled) {
                VStack(alignment: .leading, spacing: 2) {
                    Text(verbatim: "課金の制限を無効にする(デモ用・回数と有料機能)")
                    Text(verbatim: DebugKnob.Effect.immediate.label).font(.footnote).foregroundStyle(.secondary)
                }
            }
            .tint(Theme.Colors.brandA)
            .accessibilityIdentifier("debugMenu.pro.limitDisabled")
            // 設定の「つないだサービス」から移設(#2079・#2001)
            Button {
                Task {
                    let (d, text) = await DebugExtDenial.call(services.external)
                    message = text
                    denial = d
                }
            } label: {
                Text(verbatim: "試験用のサービスを呼ぶ")
            }
            .accessibilityIdentifier("debugMenu.extCall")
            #else
            Text(verbatim: "なし")
                .foregroundStyle(.secondary)
            #endif
        } header: {
            Text(verbatim: "開発用の操作")
        } footer: {
            if let message {
                Text(verbatim: message).foregroundStyle(Theme.Colors.warning)
            }
        }
    }

    // MARK: - 元に戻す / 終了する

    private var resetSection: some View {
        Section {
            Button(role: .destructive) { confirmReset = true } label: {
                Label { Text(verbatim: "全部元に戻す") } icon: { Image(systemName: "arrow.counterclockwise") }
            }
            .accessibilityIdentifier("debugMenu.reset")
            if changedRelaunchKnob {
                // 起動時に 1 回だけ読む値を変えたので、開き直すまで効かない
                Button(role: .destructive) { exit(0) } label: {
                    Label { Text(verbatim: "アプリを終了する") } icon: { Image(systemName: "power") }
                }
                .accessibilityIdentifier("debugMenu.quit")
            }
        } footer: {
            if changedRelaunchKnob {
                Text(verbatim: "起動し直しが要る項目を変えました。終了して開き直すと効きます。")
            }
        }
    }
}

/// つまみ 1 行。宣言(`DebugKnob`)だけ見て描くので、子の Issue は `DebugKnobCatalog.all` に足すだけでよい。
private struct DebugKnobRow: View {
    let knob: DebugKnob
    let onChange: (DebugKnob) -> Void

    @State private var text = ""
    @State private var isOn = false

    private var source: DebugKnobs.Source { DebugKnobs.source(knob.name) }
    /// 起動時の環境変数で固定されている行は、メニューから変えても効かないので押せなくする
    private var lockedByEnvironment: Bool { source == .environment }

    var body: some View {
        VStack(alignment: .leading, spacing: Theme.Spacing.xs) {
            control
            Text(verbatim: [knob.name, knob.effect.label, knob.note, lockedByEnvironment ? "環境変数で固定中" : nil]
                .compactMap { $0 }.joined(separator: " ・ "))
                .font(.footnote)
                .foregroundStyle(.secondary)
        }
        .disabled(lockedByEnvironment)
        .accessibilityIdentifier("debugMenu.knob.\(knob.name)")
        .onAppear(perform: load)
    }

    @ViewBuilder
    private var control: some View {
        switch knob.kind {
        case .toggle:
            Toggle(isOn: Binding(get: { isOn }, set: { isOn = $0; DebugKnobs.save($0, for: knob.name); onChange(knob) })) {
                Text(verbatim: knob.title)
            }
            .tint(Theme.Colors.brandA)
        case .text(let placeholder):
            VStack(alignment: .leading, spacing: 2) {
                Text(verbatim: knob.title)
                TextField(text: Binding(get: { text }, set: { text = $0; DebugKnobs.save($0, for: knob.name); onChange(knob) })) {
                    Text(verbatim: placeholder)
                }
                .textInputAutocapitalization(.never)
                .autocorrectionDisabled()
                .font(.footnote.monospaced())
            }
        case .choice(let options):
            Picker(selection: Binding(get: { text }, set: { text = $0; DebugKnobs.save($0, for: knob.name); onChange(knob) })) {
                Text(verbatim: "既定").tag("")
                ForEach(options, id: \.self) { Text(verbatim: $0).tag($0) }
            } label: {
                Text(verbatim: knob.title)
            }
        }
    }

    private func load() {
        let saved = DebugKnobs.string(knob.name)
        text = saved ?? ""
        if case .toggle(let fallback) = knob.kind {
            isOn = saved.flatMap(DebugKnobs.truthy) ?? fallback
        }
    }
}

#Preview {
    let services = BuildConfig.make(environment: .mock)
    NavigationStack {
        DebugMenuView()
            .environment(AuthSession(service: services.auth))
            .environment(\.services, services)
    }
}
#endif
