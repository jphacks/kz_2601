package jp.kz2601.metamon.desktop.store

import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.dsl.records.RecordRetention
import jp.kz2601.metamon.services.consent.KeyValueStore
import java.time.Instant

/** 確認した保存期間(appId → entity → 日数)。端末ごと。 */
interface RetentionConsent {
    fun days(appId: String, entity: String): Int?

    fun set(appId: String, entity: String, days: Int)

    /** すべて忘れる(後片付け)。 */
    fun clear()
}

/** [KeyValueStore] に `retention.consent.<appId>.<entity>` で覚える(Android の KeyValueRetentionConsent と同じ鍵)。 */
class KeyValueRetentionConsent(private val store: KeyValueStore) : RetentionConsent {
    override fun days(appId: String, entity: String): Int? = store.getInt(key(appId, entity))

    override fun set(appId: String, entity: String, days: Int) = synchronized(this) {
        val k = key(appId, entity)
        val keys = index()
        if (k !in keys) store.putString(INDEX, (keys + k).joinToString("\n"))
        store.putInt(k, days)
    }

    override fun clear() = synchronized(this) {
        index().forEach(store::remove)
        store.remove(INDEX)
    }

    private fun index(): List<String> = store.getString(INDEX)?.split("\n")?.filter { it.isNotEmpty() }.orEmpty()

    private fun key(appId: String, entity: String) = "retention.consent.$appId.$entity"

    private companion object {
        const val INDEX = "retention.consent.keys"
    }
}

/**
 * 記録の保存期間(#482 `Entity.retentionDays`・Android の RecordRetentionRunner)。判定は :dsl の [RecordRetention]。
 * - 利用者が確認した entity だけ消す(確認は端末ごと・appId ごと)。確認した期間より短い設定は確認し直すまで消さない
 * - private は全部、shared / public は自分が作った記録だけ(author の無い共有の記録は消さない)
 * - 消すのは呼ぶ側(DesktopSession.purgeExpired)。画面の記録から消すと、通常の削除と同じ差として端末と送信待ちに積まれる
 */
class RetentionRunner(
    private val consent: RetentionConsent,
    private val now: () -> Instant = Instant::now,
) {
    fun confirm(appId: String, entity: Entity) {
        entity.retentionDays?.let { consent.set(appId, entity.id, it) }
    }

    fun consentedDays(appId: String, entity: String): Int? = consent.days(appId, entity)

    /** 確認が要る entity(DSL の順)。 */
    fun pending(appId: String, models: List<Entity>): List<Entity> =
        RecordRetention.pending(models, models.mapNotNull { e -> consent.days(appId, e.id)?.let { e.id to it } }.toMap())

    /**
     * 期限切れで消してよい記録(entity → id)。[rows] は端末の行(作った時刻・作った人が要る)。
     * サインインしていなければ消さない(前の人の行を次の人の送信待ちに積まない)。
     */
    fun expired(appId: String, models: List<Entity>, rows: Map<String, List<Record>>, uid: String?): Map<String, List<String>> {
        if (uid.isNullOrEmpty()) return emptyMap()
        val at = now()
        return models.mapNotNull { entity ->
            val days = RecordRetention.activeDays(entity, consent.days(appId, entity.id)) ?: return@mapNotNull null
            val gone = RecordRetention.expired(rows[entity.id].orEmpty(), entity, days, at, uid).map { it.id }
            if (gone.isEmpty()) null else entity.id to gone
        }.toMap()
    }
}
