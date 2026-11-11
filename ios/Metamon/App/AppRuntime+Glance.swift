import Foundation
import MetamonDSL
import MetamonServices

/// アプリの一覧のひと目の数字(link-10)。どの数字を出すかはアプリごとにサーバー(`glanceStat`)に置き、iPhone と Mac で同じにする。
extension AppRuntime {
    /// 端末の中だけの読み口(DSL キャッシュとローカル DB)。記録の API は呼ばない。
    var glanceSource: AppGlanceSource {
        let sync = recordSync
        return AppGlanceSource(
            document: { DocumentCache.shared.load(appId: $0)?.dsl },
            cached: { appId, entity in await sync.cached(appId: appId, entity: entity) }
        )
    }

    /// カードが見えた時に呼ぶ。使用中のアプリは画面の記録、ほかは端末の中の記録で数える。
    func loadGlance(for app: AppSummary, store: AppGlanceStore = .shared) async {
        if app.id == document.appId {
            store.update(appId: app.id, dsl: dsl, records: records, setting: app.glanceStat)
        } else {
            await store.load(appId: app.id, setting: app.glanceStat, source: glanceSource)
        }
    }

    /// 一覧に出す数字を選び直す(nil = 既定・`AppGlance.hidden` = 出さない)。先に一覧へ反映し、保存に失敗したら戻す。
    func setGlanceStat(_ app: AppSummary, to statId: String?, store: AppGlanceStore = .shared) async {
        guard let i = apps.firstIndex(where: { $0.id == app.id }) else { return }
        let before = apps[i].glanceStat
        let next = statId?.isEmpty == false ? statId : nil
        guard before != next else { return }
        apps[i].glanceStat = next
        store.invalidate(app.id)
        await loadGlance(for: apps[i], store: store)
        do {
            _ = try await services.appConfig.setAppGlance(appId: app.id, glanceStat: next ?? "")
        } catch {
            print("[Metamon] setAppGlance failed: \(error)")
            if let j = apps.firstIndex(where: { $0.id == app.id }) {
                apps[j].glanceStat = before
                store.invalidate(app.id)
                await loadGlance(for: apps[j], store: store)
            }
            toast = ErrorCopy.failure(L10n.tr("一覧に出す数字を保存できませんでした"), error, context: "set app glance")
        }
    }
}
