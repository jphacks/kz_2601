package jp.kz2601.metamon.runtime

import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.RecordRetention
import jp.kz2601.metamon.services.consent.KeyValueStore
import jp.kz2601.metamon.services.local.RecordSyncEngine
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import java.time.Instant

/**
 * 記録の保存期間(#482 `Entity.retentionDays`・iOS の `AppRuntime+Retention`)。判定は :dsl の [RecordRetention]。
 *
 * - 設定された期間は、ユーザーが確認するまで消さない(消した記録は戻せない)。確認は端末ごと・appId ごとに [RetentionConsentStore]
 * - 消す時機([AppRuntime] が呼ぶ): DSL を当てた時(起動・切替・生成・ホットリロード)・前面に戻った時・記録を読んだ後・保存の後
 * - 消すのは private は全部、shared / public は自分が作った記録だけ。削除は通常の削除と同じ同期の列([RecordSyncEngine.delete])でサーバーにも送る
 * - backend は記録の期限切れを消さない(`cmd/purge` はアカウントと feedback だけ)ので、消すのは端末だけ
 */
class RecordRetentionRunner(
    private val recordSync: RecordSyncEngine,
    private val consent: RetentionConsentStore,
    private val currentUserId: () -> String?,
    private val now: () -> Instant = Instant::now,
) {
    private val _prompt = MutableStateFlow<RetentionPrompt?>(null)

    /** 出している確認(null = 出していない)。 */
    val prompt: StateFlow<RetentionPrompt?> = _prompt.asStateFlow()

    /** 「あとで」にした `appId/entity=days`。このプロセスの間は聞き直さない。 */
    private val deferred = mutableSetOf<String>()

    /** 消している最中の `appId/entity`。同時に走らせない(同じ行を 2 回消さない)。 */
    private val purging = mutableSetOf<String>()

    /** 今の DSL で確認が要る entity があれば確認を出す(無ければ閉じる)。 */
    fun note(appId: String, version: Int, entities: List<Entity>) {
        val pending = RecordRetention.pending(entities, consented(appId, entities)).filter {
            synchronized(deferred) { RetentionPrompt.key(appId, it.id, it.retentionDays ?: 0) !in deferred }
        }
        _prompt.value = if (pending.isEmpty()) null else RetentionPrompt.of(appId, version, pending)
    }

    /** 「自動で消す」: 確認した期間を覚えて確認を閉じる。覚えた確認を返す(呼ぶ側が続けて消す)。 */
    fun confirm(): RetentionPrompt? {
        val p = _prompt.value ?: return null
        _prompt.value = null
        for (item in p.items) consent.set(p.appId, item.entity, item.days)
        return p
    }

    /** 設定などから 1 つの entity だけ確認する。 */
    fun confirm(appId: String, entity: Entity) {
        val days = entity.retentionDays ?: return
        consent.set(appId, entity.id, days)
        _prompt.value = _prompt.value?.let { p -> p.copy(items = p.items.filterNot { it.entity == entity.id }).takeIf { it.items.isNotEmpty() } }
    }

    /** 「あとで」: このプロセスの間は聞き直さない(消さない)。 */
    fun defer() {
        val p = _prompt.value ?: return
        _prompt.value = null
        synchronized(deferred) { p.items.forEach { deferred += RetentionPrompt.key(p.appId, it.entity, it.days) } }
    }

    /** 「自動で消すのをやめる」を選んだ時(指示文を送るのは呼ぶ側)。確認だけ閉じる。 */
    fun dismiss() {
        _prompt.value = null
    }

    /** 確認済みの期間がある entity すべての期限切れを消す。消した entity の id を返す。 */
    suspend fun purgeAll(appId: String, entities: List<Entity>): List<String> =
        entities.filter { it.retentionDays != null && purge(appId, it) > 0 }.map { it.id }

    /** 1 つの entity の期限切れを消す(ローカル DB の内容で判定)。消した件数。 */
    suspend fun purge(appId: String, entity: Entity): Int {
        val days = RecordRetention.activeDays(entity, consent.days(appId, entity.id)) ?: return 0
        val key = "$appId/${entity.id}"
        if (!synchronized(purging) { purging.add(key) }) return 0
        try {
            // 削除待ち・取り消し待ちの行は cached に出ない(通常の削除と重ねない)
            val rows = recordSync.cached(appId, entity.id).map { it.toDsl() }
            val gone = RecordRetention.expired(rows, entity, days, now(), currentUserId())
            for (r in gone) recordSync.delete(appId, entity.id, r.id)
            return gone.size
        } finally {
            synchronized(purging) { purging.remove(key) }
        }
    }

    private fun consented(appId: String, entities: List<Entity>): Map<String, Int> =
        entities.mapNotNull { e -> consent.days(appId, e.id)?.let { e.id to it } }.toMap()
}

/** 確認した期間(appId → entity → 日数)。端末ごと(iOS は UserDefaults の `retention.consent.<appId>`)。 */
interface RetentionConsentStore {
    fun days(appId: String, entity: String): Int?

    fun set(appId: String, entity: String, days: Int)
}

/** [KeyValueStore](SharedPreferences)に `retention.consent.<appId>.<entity>` で覚える。 */
class KeyValueRetentionConsent(private val store: KeyValueStore) : RetentionConsentStore {
    override fun days(appId: String, entity: String): Int? = store.getInt(key(appId, entity))

    override fun set(appId: String, entity: String, days: Int) = store.putInt(key(appId, entity), days)

    private fun key(appId: String, entity: String) = "retention.consent.$appId.$entity"
}

/** 確認ダイアログの中身(iOS の `RetentionPrompt`)。 */
data class RetentionPrompt(val appId: String, val version: Int, val items: List<Item>) {
    data class Item(val entity: String, val name: String, val days: Int, val isShared: Boolean)

    val id: String get() = "$appId#$version#" + items.joinToString(",") { "${it.entity}=${it.days}" }

    /** 「支出」は30日 … たった記録を自動で消します。 */
    fun message(english: Boolean): String {
        val shared = items.any { it.isShared }
        if (english) {
            val parts = items.joinToString(", ") { "${it.name}: ${RecordRetention.periodLabel(it.days, english = true)}" }
            return "Entries older than the period will be deleted automatically ($parts). Deleted entries cannot be restored." +
                if (shared) " For shared entries, only the ones you added are deleted." else ""
        }
        val parts = items.joinToString("、") { "「${it.name}」は${RecordRetention.periodLabel(it.days)}" }
        return parts + "たった記録を自動で消します。消えた記録は元に戻せません。" + if (shared) "共有の記録は自分が追加したものだけが消えます。" else ""
    }

    /** 「自動で消すのをやめる」で送る指示文(声で言うのと同じ・iOS の `removeRetention`)。 */
    fun stopInstruction(english: Boolean): String = if (english) "Stop deleting entries automatically" else "記録を自動で消すのをやめて"

    companion object {
        fun of(appId: String, version: Int, entities: List<Entity>) = RetentionPrompt(
            appId, version,
            entities.mapNotNull { e -> e.retentionDays?.let { Item(e.id, e.name, it, e.isShared) } },
        )

        fun key(appId: String, entity: String, days: Int) = "$appId/$entity=$days"
    }
}
