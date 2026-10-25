import Foundation
import MetamonServices

/// 変身一覧のカード「…」→「削除」(#180)。
/// 先に一覧から隠し(楽観的 UI)、サーバー(`DELETE /v1/apps/{appId}` = 物理削除 + シェア/掲載の取り下げ)が成功したら
/// 端末のデータ(レコード DB・未送信キュー・DSL キャッシュ・サムネ)を消す。失敗したらカードを戻してトーストで知らせる。
/// 使用中のアプリを消す時は、残りのうち最新のアプリ(無ければ初期画面)に **先に** 切り替えてからサーバーを呼ぶ
/// (消した後に同期のポーリング `GET /dsl` が走ると、サーバーが同じ appId を空のアプリとして作り直すため)。
extension AppRuntime {
    func deleteApp(_ app: AppSummary) async {
        guard !appDeletion.hidden.contains(app.id) else { return } // 二度押し
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
            toast = L10n.tr("「\(app.name)」を削除できませんでした: \(error.localizedDescription)")
            return
        }
        await recordSync.purge(appId: app.id)
        DocumentCache.shared.remove(appId: app.id)
        AppThumbnailStore.shared.remove(appId: app.id)
        chatHistoryStore.remove(appId: app.id)          // チャット履歴(#210)
        toast = L10n.tr("「\(app.name)」を削除しました")
        print("[Metamon] deleted app: \(app.id) fallback=\(String(describing: fallback))")
    }

    /// 使用中のアプリ `appId` から離れる。切り替え先を読めなかった(オフライン等)ら初期画面へ。
    private func leave(_ appId: String, to fallback: AppDeletion.Fallback) async {
        if case .app(let next) = fallback {
            await switchApp(to: next)
        }
        if document.appId == appId { startFresh(.home) }
    }
}
