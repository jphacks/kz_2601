import Foundation
import MetamonServices

/// 変身一覧の整理(#277): 名前の変更・複製・並べ替え。どれも先に一覧へ反映し(楽観的 UI)、サーバーが失敗したら戻してトーストで知らせる。
/// 名前と順番はサーバーに持つ(端末に持つと機種変更・再インストールで消えるため。#276 で uid を Apple ID に引き継げば残る)。
extension AppRuntime {
    /// 名前を変える。サーバーは `dsl.app.name` にも書くので、使用中のアプリなら同期で画面のタイトルも変わる。
    func renameApp(_ app: AppSummary, to raw: String) async {
        guard let name = AppListEdit.normalizedName(raw) else {
            toast = L10n.tr("名前は1〜\(AppListEdit.maxNameLength)文字で入力してください")
            return
        }
        guard name != app.name, let i = apps.firstIndex(where: { $0.id == app.id }) else { return }
        let before = apps[i]
        apps[i].name = name
        do {
            let saved = try await services.appConfig.renameApp(appId: app.id, name: name)
            if let j = apps.firstIndex(where: { $0.id == app.id }) {
                apps[j] = saved.withSortIndex(apps[j].sortIndex)
            }
        } catch {
            print("[Metamon] renameApp failed: \(error)")
            if let j = apps.firstIndex(where: { $0.id == app.id }) { apps[j].name = before.name }
            toast = L10n.tr("名前を変えられませんでした: \(error.localizedDescription)")
        }
    }

    /// 複製する(画面の定義だけ・記録は空)。自分で並べている時は元のすぐ後ろに置く。切り替えはしない。
    func duplicateApp(_ app: AppSummary) async {
        let name = AppListEdit.copyName(of: app.name) { L10n.tr("\($0)のコピー") }
        let order = visibleApps.map(\.id)
        do {
            let copy = try await services.appConfig.duplicateApp(appId: app.id, name: name)
            apps.append(copy)
            toast = L10n.tr("「\(copy.name)」を作りました")
            if visibleApps.contains(where: { $0.sortIndex != nil }), let at = order.firstIndex(of: app.id) {
                var next = order
                next.insert(copy.id, at: at + 1)
                await saveOrder(next, rollbackTo: apps)
            }
        } catch {
            print("[Metamon] duplicateApp failed: \(error)")
            toast = L10n.tr("「\(app.name)」を複製できませんでした: \(error.localizedDescription)")
        }
    }

    /// 一覧の `index` 番目に動かす(メニューの「左へ / 右へ」とドラッグ)。
    func moveApp(_ id: String, to index: Int) async {
        let order = visibleApps.map(\.id)
        let next = AppListEdit.moved(order, id: id, to: index)
        guard next != order else { return }
        await saveOrder(next, rollbackTo: apps)
    }

    /// 並べた順を先に一覧へ反映してからサーバーに保存する。失敗したら `rollbackTo` に戻す。
    private func saveOrder(_ ids: [String], rollbackTo previous: [AppSummary]) async {
        apps = AppListEdit.applyingOrder(ids, to: apps)
        do {
            try await services.appConfig.reorderApps(ids: ids)
        } catch {
            print("[Metamon] reorderApps failed: \(error)")
            apps = previous
            toast = L10n.tr("並び順を保存できませんでした: \(error.localizedDescription)")
        }
    }
}

private extension AppSummary {
    /// 応答に sortIndex が無い(古いサーバー等)時に手元の位置を残す。
    func withSortIndex(_ fallback: Int?) -> AppSummary {
        var app = self
        if app.sortIndex == nil { app.sortIndex = fallback }
        return app
    }
}
