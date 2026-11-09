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

struct AutomationRuleRow: View {
    let rule: AutomationRule
    var isPending = false
    /// オン / オフ(`rule.enabled` を返すサーバーだけ・作成者だけがこの画面を読める)
    var onToggle: ((Bool) -> Void)?
    /// 続けて失敗している時の「直しますか?」
    var onFix: (() -> Void)?

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            info
            if let enabled = rule.enabled, let onToggle {
                Toggle(isOn: Binding(get: { enabled }, set: { onToggle($0) })) {
                    Text(enabled ? LocalizedStringKey("オン") : "オフ").font(.subheadline)
                }
                .disabled(isPending)
                .accessibilityLabel(Text("「\(rule.description)」を動かす"))
                .accessibilityHint(Text("オフにすると、このルールは動きません。ルールは消えません。"))
                .accessibilityIdentifier("automations.toggle.\(rule.id)")
            }
            if rule.suggestsFix, let onFix {
                Button(action: onFix) {
                    Label("直しますか?", systemImage: "wrench.and.screwdriver")
                }
                .buttonStyle(.borderless)
                .font(.subheadline)
                .accessibilityHint(Text("直す依頼を入力欄に入れて、設定を閉じます"))
                .accessibilityIdentifier("automations.fix.\(rule.id)")
            }
        }
        .padding(.vertical, 2)
    }

    private var isOff: Bool { rule.enabled == false }

    private var info: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack(alignment: .firstTextBaseline, spacing: 6) {
                Label {
                    Text(verbatim: rule.description)
                } icon: {
                    Image(systemName: rule.trigger == .schedule ? "clock" : "bolt")
                }
                if isOff {
                    Text("オフ")
                        .font(.caption.weight(.semibold))
                        .padding(.horizontal, 6).padding(.vertical, 2)
                        .background(.quaternary, in: Capsule())
                }
            }
            .foregroundStyle(isOff ? .secondary : .primary)
            HStack(spacing: 6) {
                Image(systemName: Self.symbol(rule.result)).foregroundStyle(Self.color(rule.result))
                Text(Self.label(rule.result))
            }
            .font(.footnote)
            if rule.stopped != nil, rule.result != .stopped {
                Text("運営がこのルールを止めています。").font(.footnote).foregroundStyle(.orange)
            }
            if let at = rule.lastRunAt {
                Text("最後に動いた: \(at.formatted(date: .abbreviated, time: .shortened))")
                    .font(.caption).foregroundStyle(.secondary)
            }
            if rule.suggestsFix {
                Text("\(rule.consecutiveFailures) 回続けてうまくいっていません。")
                    .font(.footnote).foregroundStyle(.red)
            }
        }
        .accessibilityElement(children: .combine)
    }

    static func label(_ r: AutomationRule.Result) -> LocalizedStringKey {
        switch r {
        case .none: "まだ動いていません"
        case .ok: "動きました"
        case .notifyPaused: "動きました(通知は止めています)"
        case .limit: "今日の上限に達したため止めました"
        case .serviceLimit: "サービス全体の上限に達したため止めました"
        case .servicePaused: "サービス全体で一時停止しています"
        case .plan: "今のプランでは動きません"
        case .stopped: "運営が止めています"
        case .changed: "アプリが変わったため動きませんでした"
        case .unavailable: "アプリが使えない状態だったため動きませんでした"
        case .late: "遅れたため動かしませんでした"
        case .inactive: "しばらく開かれていないため休んでいます(開くと再開します)"
        case .failed: "何度か試しましたがうまくいきませんでした"
        case .disabled: "オフにしていたため動かしませんでした"
        case .other: "動きませんでした"
        }
    }

    static func symbol(_ r: AutomationRule.Result) -> String {
        switch r {
        case .none: "circle.dashed"
        case .ok: "checkmark.circle.fill"
        case .notifyPaused, .inactive, .late: "pause.circle.fill"
        case .disabled: "pause.circle"
        case .failed, .other, .unavailable, .changed: "exclamationmark.triangle.fill"
        case .limit, .serviceLimit, .servicePaused, .plan, .stopped: "hand.raised.fill"
        }
    }

    static func color(_ r: AutomationRule.Result) -> Color {
        switch r {
        case .none, .disabled: .secondary
        case .ok: .green
        case .notifyPaused, .inactive, .late: .orange
        default: .red
        }
    }
}
#endif
