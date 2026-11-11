import Foundation
import MetamonRenderer
import MetamonServices

/// 変身一覧のカード「…」→「削除」(#180)。
/// 先に一覧から隠し(楽観的 UI)、サーバー(`DELETE /v1/apps/{appId}` = 物理削除 + シェア/掲載の取り下げ)が成功したら
/// 端末のデータ(レコード DB・未送信キュー・DSL キャッシュ・サムネ)を消す。失敗したらカードを戻してトーストで知らせる。
/// 使用中のアプリを消す時は、残りのうち最新のアプリ(無ければ初期画面)に **先に** 切り替えてからサーバーを呼ぶ
/// (消した後に同期のポーリング `GET /dsl` が走ると、サーバーが同じ appId を空のアプリとして作り直すため)。
extension AppRuntime {
    /// アプリ・アカウントを削除できるか。生成中(作る・直す)は削除しない(生成の結果が消したアプリ・アカウントに書かれるのを防ぐ)。
    var canDelete: Bool { !isMorphing }

    /// 生成中に削除できない理由(削除ボタンに添える)
    static var deleteBlockedReason: String {
        L10n.pick(L10n.tr("アプリを作っている間は削除できません。"), review: L10n.tr("記録帳を用意している間は削除できません。"))
    }

    /// true = 消えた(持てるアプリの置き換え・#630 が続けるかを決める)
    @discardableResult
    func deleteApp(_ app: AppSummary) async -> Bool {
        guard canDelete else { // 確認を出した後に生成が始まった
            toast = Self.deleteBlockedReason
            return false
        }
        guard !appDeletion.hidden.contains(app.id) else { return false } // 二度押し
        let fallback = AppDeletion.fallback(deleting: app.id, current: document.appId, apps: apps)
        do {
            try await appDeletion.run(app.id, delete: {
                if let fallback { await leave(app.id, to: fallback) }
                try await services.appConfig.deleteApp(appId: app.id)
            }, onSuccess: {
                apps.removeAll { $0.id == app.id }
            })
        } catch {
            print("[Metamon] deleteApp failed: \(error)")
            toast = ErrorCopy.failure(L10n.tr("「\(app.name)」を削除できませんでした"), error, context: "delete app")
            return false
        }
        await recordSync.purge(appId: app.id)
        LiveActivities.enqueue { await LiveActivities.end(appId: app.id) } // ロック画面・Dynamic Island(#423)
        await AppNotificationCleanup.removeApp(app.id, notifications: services.notifications) // 通知(#228)とタイマー
        DocumentCache.shared.remove(appId: app.id)
        AppThumbnailStore.shared.remove(appId: app.id)
        chatHistoryStore.remove(appId: app.id)          // チャット履歴(#210)
        toast = L10n.tr("「\(app.name)」を削除しました")
        print("[Metamon] deleted app: \(app.id) fallback=\(String(describing: fallback))")
        return true
    }

    /// 使用中のアプリ `appId` から離れる。切り替え先を読めなかった(オフライン等)ら初期画面へ。
    private func leave(_ appId: String, to fallback: AppDeletion.Fallback) async {
        if case .app(let next) = fallback {
            await switchApp(to: next)
        }
        if document.appId == appId { startFresh(.home) }
    }
}
