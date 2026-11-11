package jp.kz2601.metamon.runtime

import jp.kz2601.metamon.dsl.records.Record

/**
 * 通知の予約・条件の評価に渡す記録(#1124・iOS の `recordSync.cached`)。
 * - まだ読み込んでいない entity は端末内 DB の記録で埋める(画面を開いていない entity でも評価できる)。
 * - 読み込みに失敗した entity は、端末内 DB に記録があればそれを使い、無ければ渡さない。
 *   空と分からないまま評価すると前回を「偽」で覚え、後で読めた時に 1 回誤って鳴るため。
 *   渡さない entity があると [jp.kz2601.metamon.notifications.LocalNotifications.onRecordsChanged] は評価しない。
 */
internal object NotificationRecords {
    suspend fun fill(
        entities: Collection<String>,
        loaded: Map<String, List<Record>>,
        failed: Set<String>,
        cached: suspend (entity: String) -> List<Record>,
    ): Map<String, List<Record>> {
        val out = loaded.filterKeys { it !in failed }.toMutableMap()
        for (entity in entities.toSet()) {
            if (entity in out) continue
            val local = cached(entity)
            if (entity !in failed || local.isNotEmpty()) out[entity] = local
        }
        return out
    }
}
