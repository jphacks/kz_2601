// 審査用ビルド(APP_STORE_REVIEW)では外す: 審査用の版は AI が serverRules を作らない(#614)。
#if !APP_STORE_REVIEW
import SwiftUI
import MetamonServices

/// 設定の「自動の動き」の行(#347)。serverRules があるアプリで、live の時だけ出す。
struct AutomationsSettingsRow: View {
    @Environment(AppRuntime.self) private var rt
    /// 設定シートを閉じる(この行は設定の一番上の画面にあるので、シートの dismiss)
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        if rt.dsl.serverRuleCount > 0, let client = rt.services.automations {
            NavigationLink {
                // 「直しますか?」: 依頼を入力欄に入れて設定を閉じる(送らない・「頼み方のコツ」と同じ #415)
                AutomationsView(model: AutomationsModel(appId: rt.document.appId, client: client)) { text in
                    rt.composerDraft = text
                    dismiss()
                }
            } label: {
                Label("自動の動き", systemImage: "gearshape.2")
            }
            .accessibilityIdentifier("settings.automations")
        }
    }
}

/// 自動の動きの一覧: 説明・最後に動いた時刻・結果。止まっている時は理由とお問い合わせへの導線。
struct AutomationsView: View {
    @State var model: AutomationsModel
    /// 続けて失敗しているルールを直す依頼を入力欄へ(nil = 出さない)
    var onFix: ((String) -> Void)?

    var body: some View {
        List {
            switch model.phase {
            case .loading:
                Section {
                    ForEach(0..<2, id: \.self) { _ in
                        AutomationRuleRow(rule: AutomationRule(id: "placeholder", description: "「記録」が作られた時、アプリの作成者に通知"))
                    }
                }
                .redacted(reason: .placeholder)
            case .notCreator:
                Text("自動の動きはアプリを作った人だけが見られます。").foregroundStyle(.secondary)
            case .failed(let message):
                Section {
                    Text("自動の動きを読み込めませんでした")
                    Text(verbatim: message).font(.footnote).foregroundStyle(.secondary)
                    Button("再試行") { Task { await model.load() } }
                }
            case .loaded(let a):
                loaded(a)
            }
        }
        .navigationTitle("自動の動き")
        .navigationBarTitleDisplayMode(.inline)
        .task { await model.load() }
        .refreshable { await model.load() }
        .alert(Text("切り替えられませんでした"), isPresented: Binding(get: { model.toggleError != nil }, set: { if !$0 { model.toggleError = nil } })) {
            Button("OK", role: .cancel) {}
        } message: {
            Text(verbatim: model.toggleError ?? "")
        }
    }

    @ViewBuilder
    private func loaded(_ a: AppAutomations) -> some View {
        if Self.showsStopAppeal(a) {
            Section {
                if let s = a.stopped {
                    stopNotice(Text("運営がこのアプリの自動の動きを止めています。"), until: s.until)
                }
                if let s = a.notifyPaused {
                    stopNotice(Text("通知を一時的に止めています。自動の動きは動いています。"), until: s.until)
                }
                // #346: 心当たりがない時の申し立て先(設定の「お問い合わせ」と同じ窓口)
                Link(destination: LegalDocument.contactURL) {
                    Text("止まった理由に心当たりがない時は、お問い合わせからご連絡ください")
                        .font(.footnote)
                }
                .accessibilityIdentifier("automations.stopAppeal")
            }
        }
        Section {
            if a.rules.isEmpty {
                Text("このアプリには自動の動きがありません。").foregroundStyle(.secondary)
            }
            ForEach(a.rules) { rule in
                AutomationRuleRow(
                    rule: rule,
                    isPending: model.pending.contains(rule.id),
                    onToggle: { on in Task { await model.setEnabled(on, ruleId: rule.id) } },
                    onFix: onFix.map { fix in { fix(AutomationsModel.fixRequest(for: rule)) } }
                )
            }
        }
        if let q = a.quota {
            Section {
                LabeledContent("今日の実行") {
                    Text(verbatim: "\(q.used) / \(q.limit)").monospacedDigit()
                }
            } footer: {
                Text("日本時間の 0 時に 0 に戻ります。")
            }
        }
        if Self.needsContact(a), !Self.showsStopAppeal(a) { // 止めた時は上の申し立ての行で足りる
            Section {
                Link(destination: LegalDocument.contactURL) {
                    Label("お問い合わせ", systemImage: "envelope")
                }
                .accessibilityIdentifier("automations.contact")
            } footer: {
                Text("止まっている理由について確かめたい時は、お問い合わせください。")
            }
        }
    }

    private func stopNotice(_ text: Text, until: Date?) -> some View {
        VStack(alignment: .leading, spacing: 4) {
            Label { text } icon: { Image(systemName: "pause.circle.fill").foregroundStyle(.orange) }
            if let until {
                Text("\(until.formatted(date: .abbreviated, time: .shortened)) に自動で解除されます。")
                    .font(.footnote).foregroundStyle(.secondary)
            }
        }
    }

    /// 運営が何かを止めている(アプリ全体・通知・どれかのルール)時だけ、停止の案内の下に申し立ての行を出す(#346)。
    static func showsStopAppeal(_ a: AppAutomations) -> Bool {
        a.stopped != nil || a.notifyPaused != nil || a.rules.contains { $0.stopped != nil || $0.result == .stopped }
    }

    /// 運営の停止・止まった結果がある時だけ、お問い合わせへの導線を出す。
    static func needsContact(_ a: AppAutomations) -> Bool {
        a.stopped != nil || a.notifyPaused != nil || a.rules.contains { $0.stopped != nil || $0.result.isProblem }
    }
}
#endif
