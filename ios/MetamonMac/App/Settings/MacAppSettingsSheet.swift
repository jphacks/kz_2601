import SwiftUI
import MetamonDSL
import MetamonServices
import MetamonRenderer

/// 中央のアプリの設定(#979)。iOS の設定シートのアプリごとの節(記録の自動削除 #482・自動の動き #347)を Mac に置く。
/// ⌘, の設定ウインドウはアプリに属さない(#1219 で置き場所は未決)ので、ツールバーの「このアプリの設定」から開くシートにした。
struct MacAppSettingsSheet: View {
    let appId: String
    let appName: String
    let dsl: AppDSL
    /// 記録を端末に残すアプリ(作ったアプリ)。同梱の例の記録は一時フォルダなので自動削除の節を出さない
    var keepsRecords: Bool
    /// 自動の動きを出すか(サーバーにある作ったアプリ)。live の時だけ(`MacServices.automations`)
    var isOnServer: Bool
    /// 保存期間を確認した(開いているランタイムが消す)
    var onConfirmRetention: () -> Void
    /// 直す依頼。send = true ならすぐ作る(自動で消すのをやめる)、false なら入力欄に入れるだけ(自動の動きの「直しますか?」)
    var onRequest: (_ text: String, _ send: Bool) -> Void
    var onClose: () -> Void

    @State private var consented: [String: Int] = [:]

    private var retentionEntities: [Entity] {
        keepsRecords ? dsl.entities.filter { $0.retentionDays != nil } : []
    }

    private var showsAutomations: Bool {
        #if APP_STORE_REVIEW
        false
        #else
        isOnServer && dsl.serverRuleCount > 0 && MacServices.shared.automations != nil
        #endif
    }

    /// 他の人の行動で届く通知の同意(#339・#982)を出しうるか。作成者かどうかは節の中で確かめる
    private var mayShowPushConsent: Bool {
        #if APP_STORE_REVIEW
        false
        #else
        isOnServer && dsl.entities.contains(where: \.isShared) && MacServices.shared.pushDevices != nil
        #endif
    }

    var body: some View {
        VStack(spacing: 0) {
            Form {
                if retentionEntities.isEmpty && !showsAutomations && !mayShowPushConsent {
                    Text("このアプリには、ここで変えられる設定がありません。")
                        .macSecondaryText()
                }
                if !retentionEntities.isEmpty { retentionSection }
                #if !APP_STORE_REVIEW
                if showsAutomations, let automations = MacServices.shared.automations {
                    MacAutomationsSection(model: AutomationsModel(appId: appId, client: automations)) { onRequest($0, false) }
                }
                if mayShowPushConsent { MacPushConsentSection(appId: appId, dsl: dsl) }
                #endif
            }
            .formStyle(.grouped)
            Divider()
            HStack {
                Spacer()
                Button("閉じる", action: onClose)
                    .keyboardShortcut(.cancelAction)
                    .help("このアプリの設定を閉じる")
            }
            .padding(12)
        }
        .frame(minWidth: 460, idealWidth: 520, minHeight: 320, idealHeight: 480)
        .navigationTitle(Text("「\(appName)」の設定"))
        .onAppear { consented = RetentionConsent.days(appId: appId) }
        .accessibilityIdentifier("appSettings.sheet")
    }

    /// iOS の `RetentionSettingsSection` と同じ中身。確認は端末ごと(この Mac で確認するまで消さない)
    private var retentionSection: some View {
        let english = LanguagePreference.shared.code == "en"
        return Section {
            ForEach(retentionEntities, id: \.id) { e in
                let days = e.retentionDays ?? 0
                let active = RecordRetention.activeDays(e, consented: consented[e.id]) != nil
                VStack(alignment: .leading, spacing: 4) {
                    LabeledContent(e.name) {
                        Text(english ? RecordRetention.periodLabel(days: days, english: true) : RecordRetention.periodLabel(days: days) + "で消す")
                            .monospacedDigit()
                    }
                    HStack(spacing: 12) {
                        if !active {
                            Button("確認して自動で消す") {
                                RetentionConsent.set(days, appId: appId, entity: e.id)
                                consented = RetentionConsent.days(appId: appId)
                                onConfirmRetention()
                            }
                            .buttonStyle(.link)
                            .help("期間を過ぎた記録をこの Mac で自動で消します")
                        }
                        Button("自動で消すのをやめる", role: .destructive) {
                            onRequest(MacPostBuildNotices.stopRetentionInstruction(entityName: e.name), true)
                        }
                        .buttonStyle(.link)
                        .help("記録を自動で消さないようにアプリを直します")
                    }
                    .font(.callout)
                }
                .accessibilityElement(children: .contain)
                .accessibilityIdentifier("appSettings.retention.\(e.id)")
            }
        } header: {
            Text("記録の自動削除")
        } footer: {
            Text(Self.retentionFooter(entities: retentionEntities)).macSecondaryText()
        }
    }

    static func retentionFooter(entities: [Entity]) -> String {
        let base = L10n.tr("期間を過ぎた記録は、開いた時などに自動で消えます。消えた記録は元に戻せません。")
        return entities.contains(where: \.isShared) ? base + L10n.tr("共有の記録は自分が追加したものだけが消えます。") : base
    }
}

#if !APP_STORE_REVIEW
/// 自動の動きの一覧とオン / オフ(#347・iOS の `AutomationsView` の Mac 版)。続けて失敗しているルールには「直しますか?」
struct MacAutomationsSection: View {
    @State var model: AutomationsModel
    var onFix: (String) -> Void

    var body: some View {
        Section {
            switch model.phase {
            case .loading:
                ProgressView().frame(maxWidth: .infinity)
            case .notCreator:
                Text("自動の動きはアプリを作った人だけが見られます。").macSecondaryText()
            case .failed(let message):
                Text("自動の動きを読み込めませんでした")
                Text(verbatim: message).font(.footnote).macSecondaryText()
                Button("再試行") { Task { await model.load() } }
                    .help("自動の動きを読み込み直す")
            case .loaded(let a):
                if a.stopped != nil {
                    Label("運営がこのアプリの自動の動きを止めています。", systemImage: "pause.circle.fill")
                }
                if a.notifyPaused != nil {
                    Label("通知を一時的に止めています。自動の動きは動いています。", systemImage: "pause.circle.fill")
                }
                if a.rules.isEmpty {
                    Text("このアプリには自動の動きがありません。").macSecondaryText()
                }
                ForEach(a.rules) { rule in
                    AutomationRuleRow(rule: rule,
                                      isPending: model.pending.contains(rule.id),
                                      onToggle: { on in Task { await model.setEnabled(on, ruleId: rule.id) } },
                                      onFix: { onFix(AutomationsModel.fixRequest(for: rule)) })
                }
                if let q = a.quota {
                    LabeledContent("今日の実行") { Text(verbatim: "\(q.used) / \(q.limit)").monospacedDigit() }
                }
                if a.stopped != nil || a.notifyPaused != nil || a.rules.contains(where: { $0.stopped != nil || $0.result.isProblem }) {
                    Link("お問い合わせ", destination: LegalDocument.contactURL)
                        .help("止まっている理由について確かめる")
                }
            }
        } header: {
            HStack {
                Text("自動の動き")
                Spacer()
                Button { Task { await model.load() } } label: { Image(systemName: "arrow.clockwise") }
                    .buttonStyle(.borderless)
                    .help("自動の動きを読み込み直す")
                    .accessibilityLabel(Text("読み込み直す"))
            }
        }
        .task { await model.load() }
        .alert(Text("切り替えられませんでした"), isPresented: Binding(get: { model.toggleError != nil }, set: { if !$0 { model.toggleError = nil } })) {
            Button("OK", role: .cancel) {}
        } message: {
            Text(verbatim: model.toggleError ?? "")
        }
    }
}
#endif
