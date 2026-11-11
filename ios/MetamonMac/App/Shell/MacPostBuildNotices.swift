import SwiftUI
import Observation
import MetamonServices
import MetamonRenderer
import MetamonDSL

/// 中央の列の上に 1 つずつ出すお知らせ(#1058)。iOS と同じ順: 完成の後のお知らせ(`RootView` の要約など)→
/// 新着(`NewItemsHost`・要約が消えてから)→ 上限に近い(`UsageNoticeHost`・新着が消えてから)。
enum MacTopNotice: Equatable {
    case postBuild(PostBuildNotice)
    case newItems
    case usage
}

/// 完成の後のお知らせ(H7・#968)を Mac で 1 つずつ出す。順番は iOS と同じ `PostBuildNotice`(Shared)。
/// 今の Mac にあるのは要約(「元に戻す / やり直す」付き)→ 公開の確認(#253)→ 保存期間の確認(#482)→ 最初のアプリの後の直し方の例(#1278)。
/// 引き継ぎは機能が Mac に来たら `pending` に足す(順番はここで決まる)。
/// 新着(#268)・上限に近い(#317)はその後、サービスのお知らせ(#333)は別の段で常に一番上に出す(iOS と同じ・#1058)。
/// 「見た・閉じた」の記録は iOS と同じキー(UserDefaults)で、サインアウトの `MacLocalWipe` で消える。
@MainActor
@Observable
final class MacPostBuildNotices {
    /// 新着(#268)。「新着 N 件」
    struct NewItems: Identifiable, Equatable {
        let id = UUID()
        var appId: String
        var count: Int
    }

    var newItems: NewItems?
    /// 上限に近い・達した(#317)。1 つのアカウント・リソースにつき 1 日 1 回(`UsageNoticeLog`)
    var usage: UsageNotice?
    /// appconfig `GET /v1/client-config` のお知らせ(#333)。閉じたかは `serviceDismissals` で見る
    private(set) var service: ServiceNotice?
    /// @Observable に閉じたことを伝える(UserDefaults は追跡されない)
    private var dismissedServiceID: String?

    @ObservationIgnored let serviceDismissals: NoticeDismissals
    @ObservationIgnored let usageLog: UsageNoticeLog
    @ObservationIgnored let lastOpened: LastOpenedStore

    init(defaults: UserDefaults = .standard) {
        serviceDismissals = NoticeDismissals(defaults: defaults)
        usageLog = UsageNoticeLog(defaults: defaults)
        lastOpened = LastOpenedStore(defaults: defaults)
    }

    /// 要約のお知らせ。action はアプリの版の「元に戻す / やり直す」
    struct Summary: Identifiable, Equatable {
        enum Action: Equatable { case undo, redo }
        let id = UUID()
        var appId: String
        var text: String
        var action: Action?
        /// 出す前に読み上げ済み(会話のシートの完成で読んだ)。バーでは読み直さない
        var announced = false
    }

    var summary: Summary?
    /// 最初のアプリを作った後に 1 回だけ出す「こう頼むと直せます」(iOS の `FirstEditHintCard`・#1278)。要約が閉じてから出る
    var showsEditHint = false

    /// 要約を出しておく秒数。iOS の要約バナー(RootView)と同じ: 文字数に応じて 3〜5 秒・ボタン付きは 6 秒以上・VoiceOver の時は長く
    static func holdSeconds(_ summary: Summary) -> Double {
        MacVoiceOver.holdSeconds(max(summary.action == nil ? 3.0 : 6.0, min(5.0, Double(summary.text.count) / 12)))
    }

    /// 公開の記録に個人の情報が入った確認(#253・iOS の `PublicDataPromptHost`)。生成した appId と一緒に持つ
    struct PublicData: Identifiable, Equatable {
        var appId: String
        var prompt: PublicDataPrompt
        var id: UUID { prompt.id }
    }

    var publicData: PublicData?
    /// 記録の保存期間の確認(#482・iOS の `RetentionPromptHost`)。確認は端末ごと・アプリごと(`RetentionConsent`)
    var retention: RetentionPrompt?
    /// 「あとで」にした確認(このセッションでは聞き直さない・iOS の `retentionDeferred` と同じ)
    @ObservationIgnored private(set) var retentionDeferred: Set<String> = []

    var pending: Set<PostBuildNotice> {
        var pending: Set<PostBuildNotice> = []
        if summary != nil { pending.insert(.summary) }
        if publicData != nil { pending.insert(.publicData) }
        if retention != nil { pending.insert(.retention) }
        if showsEditHint { pending.insert(.editHint) }
        return pending
    }

    var active: PostBuildNotice? { PostBuildNotice.current(pending) }

    /// 中央の列の上に出す 1 つ。完成の後のお知らせ → 新着 → 上限(iOS と同じ順・#1058)
    var top: MacTopNotice? {
        if let active { return .postBuild(active) }
        if newItems != nil { return .newItems }
        if usage != nil { return .usage }
        return nil
    }

    /// 中央の列の上にお知らせが出ているか(出ている間はサイドバーの写しを撮らない・#1281)。
    /// サービスのお知らせは閉じるまで出続け、列を押し下げる段(写しに入らない)なので数えない
    var isShowing: Bool { top != nil }

    /// いま出ているお知らせを閉じる(閉じるボタン・Esc・時間)。閉じたら次の番のものが出る
    func dismissActive() {
        switch top {
        case .postBuild(.summary): summary = nil
        case .postBuild(.publicData): publicData = nil // 閉じる = 公開のまま(何も送らない)
        case .postBuild(.retention): deferRetention()
        case .postBuild(.editHint): showsEditHint = false
        case .newItems: newItems = nil
        case .usage: usage = nil
        default: break
        }
    }

    // MARK: - 公開の確認(#253)

    enum PublicDataChoice { case keep, remove, makePrivate }

    /// 生成の結果に公開の個人の情報があれば確認を予約する(無ければ下げる)
    func notePublicData(_ fields: [PublicPersonalField], appId: String, version: Int) {
        publicData = PublicDataPrompt(fields: fields, version: version).map { PublicData(appId: appId, prompt: $0) }
    }

    /// ボタンの答え。直す指示文を返す(送るのは呼ぶ側)。公開のまま・別のアプリ / 版に変わっていたら nil(何も送らない・iOS と同じ)
    func resolvePublicData(_ choice: PublicDataChoice, currentAppId: String?, currentVersion: Int?,
                           english: Bool = LanguagePreference.shared.code == "en") -> String? {
        guard let notice = publicData else { return nil }
        publicData = nil
        guard choice != .keep, notice.appId == currentAppId, notice.prompt.version == currentVersion else { return nil }
        return choice == .remove ? notice.prompt.removeInstruction(english: english) : notice.prompt.privateInstruction(english: english)
    }

    // MARK: - 保存期間(#482)

    /// アプリを開いた・版が変わった時に、確認が要る entity があれば確認を出す(確認済み・あとでにした分は出さない)
    func noteRetention(appId: String, version: Int, dsl: AppDSL, defaults: UserDefaults = .standard) {
        let consented = RetentionConsent.days(appId: appId, defaults: defaults)
        let pending = RecordRetention.pending(dsl, consented: consented)
            .filter { !retentionDeferred.contains(RetentionPrompt.key(appId: appId, entity: $0)) }
        retention = pending.isEmpty ? nil : RetentionPrompt(appId: appId, version: version, entities: pending)
    }

    /// 「自動で消す」: 確認した期間を覚える(消すのは `MacRetentionPurge`)。確認した appId を返す
    @discardableResult
    func confirmRetention(defaults: UserDefaults = .standard) -> String? {
        guard let prompt = retention else { return nil }
        retention = nil
        for item in prompt.items { RetentionConsent.set(item.days, appId: prompt.appId, entity: item.entity, defaults: defaults) }
        return prompt.appId
    }

    /// 「あとで」: このセッションでは聞き直さない(消さない)。アプリの設定から確認できる
    func deferRetention() {
        guard let prompt = retention else { return }
        retention = nil
        for item in prompt.items { retentionDeferred.insert(RetentionPrompt.key(appId: prompt.appId, entity: item.entity, days: item.days)) }
    }

    /// 「自動で消すのをやめる」の指示文(iOS の `removeRetention` と同じ)。送るのは呼ぶ側
    func stopRetention(english: Bool = LanguagePreference.shared.code == "en") -> String? {
        guard retention != nil else { return nil }
        retention = nil
        return Self.stopRetentionInstruction(entityName: nil, english: english)
    }

    static func stopRetentionInstruction(entityName: String?, english: Bool = LanguagePreference.shared.code == "en") -> String {
        if let entityName {
            return english ? "Stop deleting \(entityName) entries automatically" : "「\(entityName)」の記録を自動で消すのをやめて"
        }
        return english ? "Stop deleting entries automatically" : "記録を自動で消すのをやめて"
    }

    // MARK: - 新着(#268)

    /// アプリを開いた時(選んだ・前面に戻った)に数える。前回の時刻は数えるたびに進むので二重には出ない
    func checkNewItems(appId: String, entities: [String], uid: String?, source: (any SharedRecordStampSource)?, now: Date = .now) async {
        newItems = nil
        guard let source, let uid else { return }
        let count = await NewItemsCheck(source: source, store: lastOpened).run(appId: appId, entities: entities, uid: uid, now: now)
        guard !Task.isCancelled, let count, count > 0 else { return }
        newItems = NewItems(appId: appId, count: count)
    }

    // MARK: - 上限(#317)

    /// 候補のうち今日まだ出していない最初の 1 つを出す(出ている間は替えない)
    func offerUsage(_ candidates: [UsageNotice], uid: String?, now: Date = .now) {
        guard usage == nil, let next = candidates.first(where: { usageLog.shouldShow($0, uid: uid, now: now) }) else { return }
        usage = next
    }

    /// 見えた時点で数える(閉じても同じ日はもう出さない・iOS と同じ)
    func markUsageShown(uid: String?, now: Date = .now) {
        guard let usage else { return }
        usageLog.markShown(usage, uid: uid, now: now)
    }

    /// アプリの中の候補: そのアプリの記録・写真(作成者だけ)→ 作れるアプリの数
    static func usageCandidates(app: AppUsage?, me: MyUsage?) -> [UsageNotice] {
        UsageNoticeRules.appNotices(app) + [UsageNoticeRules.appsNotice(me)].compactMap { $0 }
    }

    // MARK: - サービスのお知らせ(#333)

    /// 出すサービスのお知らせ(閉じた id は二度と出さない)
    var visibleService: ServiceNotice? {
        guard let service, service.id != dismissedServiceID, !serviceDismissals.isDismissed(service.id) else { return nil }
        return service
    }

    func updateService(_ notice: ServiceNotice?) { service = notice }

    func checkService(checker: UpdateChecker) async {
        let result = await checker.check()
        guard !Task.isCancelled else { return }
        service = result.notice
    }

    func dismissService() {
        guard let service else { return }
        serviceDismissals.dismiss(service.id)
        dismissedServiceID = service.id
    }

    // MARK: - サインアウト(#972)

    /// `MacLocalWipe` の後: 出ているものを下げ、閉じた記録の写しも忘れる(UserDefaults は wipe が消している)
    func resetAfterWipe() {
        summary = nil
        publicData = nil
        retention = nil
        retentionDeferred = []
        showsEditHint = false
        newItems = nil
        usage = nil
        dismissedServiceID = nil
    }
}

/// 中央の列の上に出す要約のお知らせ。閉じるボタン・Esc・時間で閉じる。
struct MacSummaryNoticeBar: View {
    let summary: MacPostBuildNotices.Summary
    var canUndo: Bool
    var canRedo: Bool
    var undo: () -> Void
    var redo: () -> Void
    var dismiss: () -> Void

    var body: some View {
        HStack(spacing: 12) {
            Image(systemName: "checkmark.circle.fill").foregroundStyle(Theme.brandGradient)
            Text(summary.text)
                .font(.callout)
                .lineLimit(2)
                .textSelection(.enabled)
            switch summary.action {
            case .undo?:
                Button("元に戻す", action: undo)
                    .disabled(!canUndo)
                    .help(Text(verbatim: "⌥⌘Z"))
            case .redo?:
                Button("やり直す", action: redo)
                    .disabled(!canRedo)
                    .help(Text(verbatim: "⌥⇧⌘Z"))
            case nil:
                EmptyView()
            }
            Button(action: dismiss) { Image(systemName: "xmark") }
                .buttonStyle(.plain)
                .macSecondaryText() // コントラストを上げる時は本文の色(#1206)
                .keyboardShortcut(.cancelAction) // Esc で閉じる
                .help("閉じる")
                .accessibilityLabel("閉じる")
        }
        .padding(.horizontal, 14).padding(.vertical, 8)
        .macRaisedSurface(in: Capsule()) // 透明度を下げる・コントラストを上げる(#1206)
        .frame(maxWidth: 560)
        .macTopTransition()
        .accessibilityElement(children: .contain)
    }
}

/// 最初のアプリの後の「こう頼むと直せます」(iOS の `FirstEditHintCard`・#1278)。iOS の「マイクをタップ」を ⌘K / ⇧⌘M の案内に言い換える。
/// 例文は iOS と同じ `FirstRunTutorial.editHintExamples`。時間では消さない(閉じるか、頼んでみるまで出す)
struct MacEditHintBar: View {
    var ask: () -> Void
    var dismiss: () -> Void

    var body: some View {
        HStack(alignment: .top, spacing: 12) {
            CraneMascot(pose: .point, size: 40, sway: 3)
                .accessibilityHidden(true)
            VStack(alignment: .leading, spacing: 4) {
                Text("こう頼むと直せます").font(.headline)
                Text("⌘K で文字で、⇧⌘M で声で頼めます。")
                    .font(.callout)
                    .macSecondaryText() // コントラストを上げる時は本文の色(#1206)
                Text(FirstRunTutorial.editHintExamples.map { L10n.tr("「\($0)」") }.joined(separator: " "))
                    .font(.callout.weight(.medium))
                    .foregroundStyle(Theme.Colors.brandText)
                    .fixedSize(horizontal: false, vertical: true)
            }
            VStack(alignment: .trailing, spacing: 6) {
                Button("頼んでみる", action: ask)
                    .buttonStyle(.borderedProminent)
                    .accessibilityIdentifier("editHint.talk")
                Button("とじる", action: dismiss)
                    .keyboardShortcut(.cancelAction) // Esc で閉じる
                    .accessibilityIdentifier("editHint.close")
            }
        }
        .padding(.horizontal, 16).padding(.vertical, 12)
        .macRaisedSurface(in: RoundedRectangle(cornerRadius: 14, style: .continuous)) // #1206
        .frame(maxWidth: 560)
        .macTopTransition()
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("editHint.card")
    }
}

/// 新着(#268・iOS の `NewItemsHost`)。数秒か閉じるボタン・Esc で消える
struct MacNewItemsBar: View {
    let notice: MacPostBuildNotices.NewItems
    var dismiss: () -> Void

    var body: some View {
        HStack(spacing: 12) {
            Image(systemName: "bell.badge.fill")
                .foregroundStyle(Theme.brandGradient)
                .accessibilityHidden(true)
            VStack(alignment: .leading, spacing: 2) {
                Text("新着 \(notice.count) 件").font(.callout.weight(.semibold))
                Text("ほかの人が追加・更新しました")
                    .font(.caption)
                    .macSecondaryText()
            }
            MacNoticeCloseButton(dismiss: dismiss, cancelAction: true)
        }
        .padding(.horizontal, 14).padding(.vertical, 8)
        .macRaisedSurface(in: Capsule())
        .frame(maxWidth: 560)
        .macTopTransition()
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("newItems.banner")
    }
}

/// 上限に近い・達した(#317・iOS の `UsageNoticeBanner`)。「Pro にする」は押した時だけ課金画面を開く
struct MacUsageNoticeBar: View {
    let notice: UsageNotice
    /// nil = 「Pro にする」を出さない(Pro・審査用の版・上がらない上限)
    var upgrade: (() -> Void)?
    var dismiss: () -> Void

    var body: some View {
        HStack(spacing: 12) {
            Image(systemName: "gauge.with.dots.needle.67percent")
                .macSecondaryText()
                .accessibilityHidden(true)
            Text(UsageCopy.notice(notice))
                .font(.callout)
                .lineLimit(2)
                .accessibilityIdentifier("usage.notice.text")
            if let upgrade {
                Button("Pro にする", action: upgrade)
                    .controlSize(.small)
                    .help("Pro にすると上限が増えます")
                    .accessibilityIdentifier("usage.notice.upgrade")
            }
            MacNoticeCloseButton(dismiss: dismiss, cancelAction: true)
        }
        .padding(.horizontal, 14).padding(.vertical, 8)
        .macRaisedSurface(in: Capsule())
        .frame(maxWidth: 560)
        .macTopTransition()
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("usage.notice.\(notice.resource.rawValue)")
    }
}

/// サービスのお知らせ(#333・iOS の `ServiceNoticeBanner`)。閉じるまで出す。閉じた id は二度と出さない
struct MacServiceNoticeBar: View {
    let notice: ServiceNotice
    var dismiss: () -> Void
    @Environment(\.openURL) private var openURL

    private var isWarning: Bool { notice.severity == .warning }
    private var text: String { notice.text(languageCode: LanguagePreference.shared.code) }

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: 12) {
            Image(systemName: isWarning ? "exclamationmark.triangle.fill" : "info.circle.fill")
                .foregroundStyle(isWarning ? AnyShapeStyle(Theme.Colors.warning) : AnyShapeStyle(Theme.brandGradient))
                .accessibilityHidden(true)
            Text(verbatim: text)
                .font(.callout)
                .fixedSize(horizontal: false, vertical: true)
                .textSelection(.enabled)
                .frame(maxWidth: .infinity, alignment: .leading)
                .accessibilityLabel(isWarning ? Text("重要なお知らせ: \(text)") : Text("お知らせ: \(text)"))
            if let url = notice.linkURL {
                Button("詳しく見る") { openURL(url) }
                    .buttonStyle(.link)
                    .help(Text(verbatim: url.absoluteString))
                    .accessibilityIdentifier("notice.link")
            }
            // Esc は下の段(要約など)に譲る
            MacNoticeCloseButton(dismiss: dismiss, cancelAction: false, label: "お知らせを閉じる")
                .accessibilityIdentifier("notice.dismiss")
        }
        .padding(.horizontal, 14).padding(.vertical, 8)
        .macRaisedSurface(in: RoundedRectangle(cornerRadius: 10, style: .continuous))
        .frame(maxWidth: 560)
        .macTopTransition()
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("notice.banner")
    }
}

/// お知らせの閉じるボタン
private struct MacNoticeCloseButton: View {
    var dismiss: () -> Void
    /// Esc で閉じる(同時に 1 つだけ)
    var cancelAction: Bool
    var label: LocalizedStringKey = "閉じる"

    var body: some View {
        Button(action: dismiss) { Image(systemName: "xmark") }
            .buttonStyle(.plain)
            .macSecondaryText()
            .keyboardShortcut(cancelAction ? KeyboardShortcut.cancelAction : nil)
            .help(Text(label))
            .accessibilityLabel(Text(label))
    }
}
