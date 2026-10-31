import SwiftUI
import MetamonDSL
import MetamonRenderer
import MetamonServices

/// 記録の保存期間(#482 `Entity.retentionDays`)。
/// - 設定された期間は、ユーザーが確認するまで消さない(消した記録は戻せない)。確認は端末ごと・appId ごとに `RetentionConsent` へ
/// - 消す時機: DSL を当てた時(起動・切替・生成・ホットリロード)・前面に戻った時・サーバーから読み直した後・保存の後
/// - 消すのは `RecordRetention.expired`(private は全部、shared / public は自分が作った記録だけ)。削除は通常の削除と同じ同期キューで送る
extension AppRuntime {
    /// 今の DSL で確認が要る entity があれば確認を出し、確認済みの entity の期限切れを消す。`apply` から呼ぶ。
    func noteRetention() {
        let appId = document.appId
        let consented = RetentionConsent.days(appId: appId)
        let pending = RecordRetention.pending(dsl, consented: consented)
            .filter { !retentionDeferred.contains(RetentionPrompt.key(appId: appId, entity: $0)) }
        retentionPrompt = pending.isEmpty ? nil : RetentionPrompt(appId: appId, version: document.version, entities: pending)
        Task { await purgeAllExpired() }
    }

    /// 「自動で消す」: 確認した期間を覚えて、すぐ消す。
    func confirmRetention() async {
        guard let prompt = retentionPrompt else { return }
        retentionPrompt = nil
        guard prompt.appId == document.appId else { return }
        for item in prompt.items { RetentionConsent.set(item.days, appId: prompt.appId, entity: item.entity) }
        print("[Metamon] retention confirmed: \(prompt.items.map { "\($0.entity)=\($0.days)d" })")
        await purgeAllExpired()
    }

    /// 「あとで」: このセッションでは聞き直さない(消さない)。設定シートから確認できる。
    func deferRetention() {
        guard let prompt = retentionPrompt else { return }
        retentionPrompt = nil
        for item in prompt.items { retentionDeferred.insert(RetentionPrompt.key(appId: prompt.appId, entity: item.entity, days: item.days)) }
    }

    /// 設定シートの「自動で消す」(あとでにした分をあらためて確認する)。
    func confirmRetention(entity id: String) async {
        guard let days = dsl.entity(id)?.retentionDays else { return }
        RetentionConsent.set(days, appId: document.appId, entity: id)
        await purgeExpired(id)
    }

    /// 「自動で消すのをやめる」: 既存の修正(generate・mode edit)に指示文を送る(声で「自動で消すのをやめて」と言うのと同じ)。
    func removeRetention(entityName: String? = nil) async {
        retentionPrompt = nil
        let en = LanguagePreference.shared.code == "en"
        let text: String
        if let entityName {
            text = en ? "Stop deleting \(entityName) entries automatically" : "「\(entityName)」の記録を自動で消すのをやめて"
        } else {
            text = en ? "Stop deleting entries automatically" : "記録を自動で消すのをやめて"
        }
        await generate(text)
    }

    /// 消してよい(確認済みの)期間がある entity すべて。
    func purgeAllExpired() async {
        for e in dsl.entities where e.retentionDays != nil { await purgeExpired(e.id) }
    }

    /// 1 つの entity の期限切れを消す。ローカル DB の内容で判定し、表示中なら一覧も差し替える。
    func purgeExpired(_ entityId: String) async {
        let appId = document.appId
        guard let entity = dsl.entity(entityId),
              let days = RecordRetention.activeDays(entity, consented: RetentionConsent.days(appId: appId)[entityId]) else { return }
        let key = appId + "/" + entityId
        guard !retentionPurging.contains(key) else { return } // 同時に走らせない(同じ行を 2 回消さない)
        retentionPurging.insert(key)
        defer { retentionPurging.remove(key) }
        let rows = await recordSync.cached(appId: appId, entity: entityId)
        let gone = RecordRetention.expired(rows, in: entity, days: days, now: DSLClock.now(), uid: currentUserId)
        guard !gone.isEmpty else { return }
        for r in gone { _ = await recordSync.delete(appId: appId, entity: entityId, recordId: r.id) }
        print("[Metamon] retention: deleted \(gone.count) record(s) older than \(days)d entity=\(entityId)")
        guard records[entityId] != nil else { return } // 未表示の entity は画面が開いた時に読む
        await reloadLocal(entityId, appId: appId)
    }
}

/// 確認した期間(appId → entity → 日数)。端末ごと。アプリを消したら一緒に消える程度の情報なので UserDefaults。
enum RetentionConsent {
    private static func key(_ appId: String) -> String { "retention.consent.\(appId)" }

    static func days(appId: String, defaults: UserDefaults = .standard) -> [String: Int] {
        (defaults.dictionary(forKey: key(appId)) as? [String: Int]) ?? [:]
    }

    static func set(_ days: Int, appId: String, entity: String, defaults: UserDefaults = .standard) {
        var all = Self.days(appId: appId, defaults: defaults)
        all[entity] = days
        defaults.set(all, forKey: key(appId))
    }
}

/// 確認ダイアログの中身。
struct RetentionPrompt: Identifiable, Equatable {
    struct Item: Equatable {
        var entity: String
        var name: String
        var days: Int
        var isShared: Bool
    }

    var appId: String
    var version: Int
    var items: [Item]
    var id: String { "\(appId)#\(version)#" + items.map { "\($0.entity)=\($0.days)" }.joined(separator: ",") }

    init(appId: String, version: Int, entities: [Entity]) {
        self.appId = appId
        self.version = version
        items = entities.compactMap { e in e.retentionDays.map { Item(entity: e.id, name: e.name, days: $0, isShared: e.isShared) } }
    }

    static func key(appId: String, entity: Entity) -> String { key(appId: appId, entity: entity.id, days: entity.retentionDays ?? 0) }
    static func key(appId: String, entity: String, days: Int) -> String { "\(appId)/\(entity)=\(days)" }

    /// 「支出」は30日、「日記」は1週間 … たった記録が自動で消えます。
    func message(english: Bool) -> String {
        let parts = items.map { english ? "\($0.name): \(RecordRetention.periodLabel(days: $0.days, english: true))" : "「\($0.name)」は\(RecordRetention.periodLabel(days: $0.days))" }
        let shared = items.contains(where: \.isShared)
        if english {
            return "Entries older than the period will be deleted automatically (\(parts.joined(separator: ", "))). Deleted entries cannot be restored."
                + (shared ? " For shared entries, only the ones you added are deleted." : "")
        }
        return parts.joined(separator: "、") + "たった記録を自動で消します。消えた記録は元に戻せません。" + (shared ? "共有の記録は自分が追加したものだけが消えます。" : "")
    }
}

/// 確認ダイアログ。生成直後は音声シートが閉じる途中なので、少し待ってから出す(`PublicDataPromptHost` と同じ)。
struct RetentionPromptHost: ViewModifier {
    @Environment(AppRuntime.self) private var rt
    @State private var shown: RetentionPrompt?

    func body(content: Content) -> some View {
        content
            .task(id: rt.retentionPrompt?.id) {
                shown = nil
                guard rt.retentionPrompt != nil else { return }
                try? await Task.sleep(for: .milliseconds(900))
                guard !Task.isCancelled else { return }
                shown = rt.retentionPrompt
            }
            .alert(L10n.tr("古い記録を自動で消しますか?"), isPresented: Binding(get: { shown != nil }, set: { if !$0 { shown = nil } }),
                   presenting: shown) { _ in
                Button(L10n.tr("自動で消す"), role: .destructive) { Task { await rt.confirmRetention() } }
                    .accessibilityIdentifier("retention.confirm")
                Button(L10n.tr("自動で消すのをやめる")) { Task { await rt.removeRetention() } }
                Button(L10n.tr("あとで"), role: .cancel) { rt.deferRetention() }
            } message: { prompt in
                Text(prompt.message(english: LanguagePreference.shared.code == "en"))
            }
    }
}

/// 設定シートの「記録の自動削除」。今の期間と、確認・解除のボタン。
struct RetentionSettingsSection: View {
    @Environment(AppRuntime.self) private var rt

    var body: some View {
        let entities = rt.dsl.entities.filter { $0.retentionDays != nil }
        if !entities.isEmpty {
            let consented = RetentionConsent.days(appId: rt.document.appId)
            let english = LanguagePreference.shared.code == "en"
            Section {
                ForEach(entities, id: \.id) { e in
                    let days = e.retentionDays ?? 0
                    let active = RecordRetention.activeDays(e, consented: consented[e.id]) != nil
                    VStack(alignment: .leading, spacing: 4) {
                        LabeledContent(e.name) {
                            Text(english ? RecordRetention.periodLabel(days: days, english: true) : RecordRetention.periodLabel(days: days) + "で消す")
                                .monospacedDigit()
                        }
                        if !active {
                            Button(L10n.tr("確認して自動で消す")) { Task { await rt.confirmRetention(entity: e.id) } }
                                .font(.callout)
                                .buttonStyle(.borderless)
                        }
                        Button(L10n.tr("自動で消すのをやめる"), role: .destructive) { Task { await rt.removeRetention(entityName: e.name) } }
                            .font(.callout)
                            .buttonStyle(.borderless)
                    }
                    .accessibilityElement(children: .contain)
                    .accessibilityIdentifier("settings.retention.\(e.id)")
                }
            } header: {
                Text("記録の自動削除")
            } footer: {
                Text("期間を過ぎた記録は、開いた時などに自動で消えます。消えた記録は元に戻せません。共有の記録は自分が追加したものだけが消えます。")
            }
        }
    }
}
