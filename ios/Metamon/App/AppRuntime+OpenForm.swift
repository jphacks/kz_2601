import Foundation
import MetamonDSL
import MetamonRenderer

/// 別の生成アプリのフォームを値入りで新規に開く本体の口(#2235・openApp #1893 の下ごしらえ。DSL は変えない)。
/// 記録の貼り付け(#1891)・Mac のドラッグも同じ口を使える。開くだけで、記録は利用者が保存した時だけ入る。
/// 保存して閉じたら元のアプリに戻る。保存せずに閉じたら何も残らない(戻り先も一定時間で捨てる)。
extension AppRuntime {
    /// 保存した時に戻る先。開いたアプリ・entity・画面の新規の保存だけで使う
    struct OpenFormReturn: Equatable {
        var originAppId: String
        var appId: String
        var entityId: String
        var screen: String
        var openedAt: Date

        /// 開いてからこれより後の保存では戻らない(保存せずに閉じて、後で別の用で保存した時に飛ばさない)
        static let lifetime: TimeInterval = 30 * 60

        func matches(appId: String, entityId: String, route: Route, now: Date = Date()) -> Bool {
            self.appId == appId && self.entityId == entityId && route.screen == screen && route.recordId == nil
                && now.timeIntervalSince(openedAt) < Self.lifetime
        }
    }

    enum OpenFormOutcome: Equatable {
        /// 開いた。`skipped` は入らなかった項目
        case opened(skipped: [String])
        /// そのアプリに入力画面が無い(アプリは開いたまま)
        case noForm
        /// アプリを開けなかった(切替に失敗・トーストは switchApp が出す)
        case notOpened
    }

    /// `appId` のアプリに切り替え、`entityId` の入力画面を `values`(項目の id か名前 → 値)入りの新規で開く
    @discardableResult
    func openForm(appId: String, entityId: String, values: [String: JSONValue]) async -> OpenFormOutcome {
        let origin = document.appId
        let hadModal = showVoiceSheet || sheet != nil || fullScreen != nil
        showVoiceSheet = false
        sheet = nil
        fullScreen = nil
        if appId != document.appId { await switchApp(to: appId, afterSheetDismiss: hadModal) }
        guard document.appId == appId else { return .notOpened }
        showAppSwitcher = false
        guard case .success(let plan) = OpenFormPrefill.plan(entityId: entityId, values: values, in: dsl,
                                                            isAdmin: viewerIsAdmin == true) else {
            toast = L10n.tr("このアプリには入力の画面がありません")
            return .noForm
        }
        if hadModal && origin == appId { try? await Task.sleep(for: Self.sheetDismissDelay) }
        FormPrefill.set(plan.values)
        reveal(screen: plan.screen)
        openFormReturn = origin == appId ? nil
            : OpenFormReturn(originAppId: origin, appId: appId, entityId: entityId, screen: plan.screen, openedAt: Date())
        if !plan.skipped.isEmpty { toast = L10n.tr("\(plan.skipped.count) 項目は入りませんでした") }
        return .opened(skipped: plan.skipped)
    }

    /// 保存の直前に呼ぶ: `openForm` で開いたフォームの新規の保存なら、戻り先を取り出す(1 回だけ)
    func takeOpenFormReturn(appId: String, entityId: String, route: Route) -> OpenFormReturn? {
        guard let r = openFormReturn, r.matches(appId: appId, entityId: entityId, route: route) else { return nil }
        openFormReturn = nil
        return r
    }

    /// 保存して画面を閉じた後、元のアプリに戻る
    func returnAfterOpenForm(_ r: OpenFormReturn) async {
        try? await Task.sleep(for: Self.sheetDismissDelay)
        guard document.appId == r.appId else { return }
        await switchApp(to: r.originAppId)
    }
}
