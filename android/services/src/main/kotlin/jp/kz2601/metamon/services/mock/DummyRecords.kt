package jp.kz2601.metamon.services.mock

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.Entity
import jp.kz2601.metamon.dsl.Field
import jp.kz2601.metamon.dsl.FieldType
import jp.kz2601.metamon.dsl.Scope
import jp.kz2601.metamon.services.Record
import jp.kz2601.metamon.services.local.RecordSyncEngine
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.temporal.ChronoUnit

/**
 * entity の欄の型からそれらしいダミーの記録を作る(iOS の `DummyRecords`・見た目の監査 #131)。
 * 長い文字列・大きい金額を混ぜて崩れを炙り出す。つまみは `METAMON_SEED_DUMMY` / `_STEP_HOURS` / `_AUTHORS`。
 */
object DummyRecords {
    /** モックの 2 人目(自分 = [MockAuthService.FIXED_UID] 以外の投稿者) */
    const val OTHER_UID = "mock00-other-uid"

    fun make(
        dsl: AppDSL,
        count: Int,
        now: Instant = Instant.now(),
        stepHours: Double = 1.0,
        zone: ZoneId = ZoneId.systemDefault(),
    ): Map<String, List<Record>> = dsl.entities.associate { entity ->
        entity.id to (0 until count).map { i ->
            val data = entity.fields.filter { it.computed == null }
                .mapNotNull { f -> value(f, entity, i, dsl, now, zone)?.let { f.id to it } }.toMap()
            // 新しい順に stepHours ずつ前(`$createdAt` の並びと表示を確かめる・#250)
            val created = now.minusMillis((i * stepHours * 3_600_000).toLong()).truncatedTo(ChronoUnit.SECONDS).toString()
            val shared = entity.scope == Scope.SHARED || entity.scope == Scope.PUBLIC
            // 共有 entity: 偶数番は自分、奇数番はほかの人の投稿
            val mine = i % 2 == 0
            Record(
                id = "${entity.id}-${i + 1}",
                entity = entity.id,
                data = JsonObject(data),
                author = if (!shared) null else if (mine) MockAuthService.FIXED_UID else "dummy-member-$i",
                authorName = if (!shared) null else if (mine) RecordSyncEngine.ANONYMOUS_NAME else OTHER_NAMES[i % OTHER_NAMES.size],
                createdAt = created,
            )
        }
    }

    /** 1 件目・3 件目… は自分、2 件目・4 件目… は別の人の投稿にする(`Field.hidden` #262 の確認)。 */
    fun alternatingAuthors(seed: Map<String, List<Record>>): Map<String, List<Record>> =
        seed.mapValues { (_, rows) ->
            rows.mapIndexed { i, r -> r.copy(author = if (i % 2 == 0) MockAuthService.FIXED_UID else OTHER_UID) }
        }

    private val OTHER_NAMES = listOf("さくら", "たけし", "匿名")
    private val SHORT_WORDS = listOf("ランチ", "電車", "カフェ", "本", "日用品", "サークル費")
    private val LONG_WORDS = listOf("駅前の新しくできたイタリアンレストランで友達と夕食", "Weekend grocery run at the big supermarket")
    private val AMOUNTS = listOf(980, 12_800, 450, 128_000, 3_200, 60)
    private val INTEGERS = listOf(3, 12, 1, 150, 8, 42)
    private val NUMBERS = listOf(65.4, 170.2, 2.5, 1200.75, 0.8, 21.3)

    /** 一覧の見出しの欄(titleField → 最初の string → 最初の欄。hidden は除く)。 */
    private fun titleField(entity: Entity): String? {
        val visible = entity.fields.filter { it.computed == null && it.hidden != true }
        return entity.titleField ?: visible.firstOrNull { it.type == FieldType.STRING }?.id ?: visible.firstOrNull()?.id
    }

    private fun value(f: Field, entity: Entity, i: Int, dsl: AppDSL, now: Instant, zone: ZoneId): JsonElement? = when (f.type) {
        FieldType.STRING ->
            if (f.id == titleField(entity)) JsonPrimitive(if (i == 2) LONG_WORDS[0] else "${f.name} ${i + 1}")
            else JsonPrimitive(SHORT_WORDS[i % SHORT_WORDS.size])
        FieldType.TEXT -> JsonPrimitive(if (i == 1) LONG_WORDS[0] + "。" + LONG_WORDS[1] else "${f.name}のメモ ${i + 1}")
        FieldType.CURRENCY -> JsonPrimitive(AMOUNTS[i % AMOUNTS.size])
        FieldType.INTEGER -> JsonPrimitive(INTEGERS[i % INTEGERS.size])
        FieldType.NUMBER -> JsonPrimitive(NUMBERS[i % NUMBERS.size])
        FieldType.BOOLEAN -> JsonPrimitive(i % 2 == 0)
        FieldType.DATE, FieldType.DATETIME -> {
            val d = now.atZone(zone).minusDays(i * 2L)
            JsonPrimitive(
                if (f.type == FieldType.DATE) d.toLocalDate().toString()
                else d.truncatedTo(ChronoUnit.SECONDS).format(DateTimeFormatter.ISO_OFFSET_DATE_TIME),
            )
        }
        FieldType.ENUM -> f.options.takeIf { it.isNotEmpty() }?.let { JsonPrimitive(it[i % it.size]) }
        FieldType.REF -> f.refEntity?.takeIf { t -> dsl.entities.any { it.id == t } }?.let { JsonPrimitive("$it-${i % 3 + 1}") }
        FieldType.REFS -> f.refEntity?.takeIf { t -> dsl.entities.any { it.id == t } }?.let { t ->
            if (i % 3 == 0) JsonArray(emptyList()) else JsonArray(listOf(JsonPrimitive("$t-1"), JsonPrimitive("$t-${i % 3 + 1}")))
        }
        // 別の端末で撮った動画・録音(この端末にファイルが無い)の見え方を確かめる(#206)
        FieldType.VIDEO -> JsonPrimitive("video:dummy-$i")
        FieldType.AUDIO -> JsonPrimitive("audio:dummy-$i|${30 + i * 7}")
        else -> null
    }
}
