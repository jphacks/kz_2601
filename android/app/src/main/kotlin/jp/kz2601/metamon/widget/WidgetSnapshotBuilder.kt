package jp.kz2601.metamon.widget

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.records.filters
import jp.kz2601.metamon.dsl.isVoiceOnly
import jp.kz2601.metamon.dsl.records.Aggregator
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.dsl.records.HiddenFields
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.dsl.records.RecordValue
import jp.kz2601.metamon.dsl.records.Track
import jp.kz2601.metamon.dsl.screen
import jp.kz2601.metamon.renderer.EntityLoadState
import jp.kz2601.metamon.renderer.components.lists.LeafData
import jp.kz2601.metamon.renderer.components.lists.StatValue
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneId
import java.util.Locale

/**
 * DSL の先頭 3 個の `stat` を集計してウィジェットの中身を作る(iOS の `WidgetSnapshotWriter.makeSnapshot`・#912)。
 * 値はアプリ内の stat と同じ [StatValue.resolve] で出すので、画面とウィジェットの数字が一致する。
 * 大きいサイズ用に、先頭の(記録を数える)stat の entity の「最近の記録」と「直近 7 日」も入れる。
 */
object WidgetSnapshotBuilder {
    const val MAX_STATS = 3
    const val MAX_RECENT = 5
    const val TREND_DAYS = 7

    private val periodOps = setOf("today", "thisWeek", "thisMonth", "thisYear")

    /** 初期状態(音声画面だけ)は null(ウィジェットは「アプリを作るとここに出ます」)。 */
    fun build(
        appId: String,
        dsl: AppDSL,
        data: LeafData,
        previous: WidgetSnapshot?,
        now: Instant = data.now,
        zone: ZoneId = FieldCoding.zone,
        locale: Locale = Locale.getDefault(),
    ): WidgetSnapshot? {
        if (dsl.isVoiceOnly) return null
        // 別のアプリの前回値は引き継がない
        val prev = previous?.takeIf { it.appId == appId }
        val components = statComponents(dsl).take(MAX_STATS)
        val stats = components.map { c ->
            val key = listOf(c.entity.orEmpty(), c.aggregate?.wire ?: "count", c.field.orEmpty(), c.label.orEmpty()).joinToString("|")
            val resolved = StatValue.resolve(c, data, locale)
            // 読み込み中は前回値(ウィジェットが一瞬「—」や 0 にならない)
            val value = if (resolved.loading) prev?.stats?.firstOrNull { it.key == key }?.value else resolved.text
            WidgetSnapshot.Stat(key, c.label.orEmpty(), value)
        }
        val base = WidgetSnapshot(appId, dsl.app.name, stats, now.toEpochMilli())

        val primary = components.firstOrNull { c -> c.entity?.let(data::entity) != null && !StatValue.isValueMode(c) } ?: return base
        val entity = data.entity(primary.entity) ?: return base
        val withTitle = base.copy(recentTitle = entity.name)
        if (data.loadState(entity.id) == EntityLoadState.LOADING && data.records(entity.id).isEmpty()) {
            // 未取得は前回分を引き継ぐ(同じ entity の時だけ)
            return if (prev?.recentTitle == entity.name) withTitle.copy(recent = prev.recent, trend = prev.trend) else withTitle
        }
        return withTitle.copy(
            recent = recentRows(entity, data, primary.field, now, locale),
            trend = trend(primary, entity, data, now, zone),
        )
    }

    /** 画面順(initialScreen → タブ順 → 残り)で `stat` を並べる(入れ子の中も)。 */
    fun statComponents(dsl: AppDSL): List<Component> {
        val order = (listOf(dsl.initialScreen) + dsl.app.tabs.map { it.screen } + dsl.screens.map { it.id }).distinct()
        return order.mapNotNull { dsl.screen(it) }
            .flatMap { s -> s.components.flatMap(::walk) }
            .filter { it.type == ComponentType.STAT }
    }

    private fun walk(c: Component): List<Component> = listOf(c) + c.children.flatMap(::walk)

    /** 新しい順(作成日時)に [MAX_RECENT] 件。hidden の field は出さない。 */
    fun recentRows(entity: Entity, data: LeafData, preferredValueField: String?, now: Instant, locale: Locale): List<WidgetSnapshot.Row> {
        val visible = entity.fields.filter { !it.hidden }
        val titleField = entity.displayTitleField?.let(entity::field)
        val valueField = preferredValueField?.let(entity::field)?.takeIf { !it.hidden && it.type.isNumeric }
            ?: visible.firstOrNull { it.type.isNumeric }
        val dateField = visible.firstOrNull { it.type.isDate }
        val rows = HiddenFields.redact(Aggregator.sorted(data.viewRecords(entity.id), null, entity), entity).take(MAX_RECENT)
        return rows.map { r ->
            val title = titleField?.let { data.display(r[it.id], it) }.orEmpty()
            val detail = if (dateField != null && !r[dateField.id].isNull) {
                data.display(r[dateField.id], dateField)
            } else {
                createdAt(r)?.let { FieldCoding.displayDate(it, withTime = false, locale = locale, now = now) }
            }
            val value = valueField?.takeIf { !r[it.id].isNull }?.let { data.display(r[it.id], it) }
            WidgetSnapshot.Row(r.id, title.ifEmpty { FieldCoding.UNAVAILABLE }, detail, value)
        }
    }

    /**
     * 先頭 stat の集計を日ごとに [TREND_DAYS] 日ぶん(今日まで)。latest / earliest と位置の記録は日ごとにできないので null。
     * 日付は entity の最初の date field(無ければ作成日時)。期間の filter(今月など)は外し、それ以外の filter は掛ける。
     */
    fun trend(c: Component, entity: Entity, data: LeafData, now: Instant, zone: ZoneId): WidgetSnapshot.Trend? {
        val op = c.aggregate?.wire
        if (Aggregator.isPick(op) || Track.isTrackAggregate(op)) return null
        val rows = data.filtered(c, c.filters.filter { it.op !in periodOps })
        val dateField = entity.fields.firstOrNull { !it.hidden && it.type.isDate }
        val today = LocalDate.ofInstant(now, zone)
        val first = today.minusDays((TREND_DAYS - 1).toLong())
        val byDay = rows.groupBy { r ->
            val d = dateField?.let { FieldCoding.date(r[it.id], zone) } ?: createdAt(r)
            d?.let { LocalDate.ofInstant(it, zone) }
        }.filterKeys { it != null && !it.isBefore(first) && !it.isAfter(today) }
        val days = (0 until TREND_DAYS).map { first.plusDays(it.toLong()) }
        val points = days.map { WidgetSnapshot.Trend.Point(it.toEpochDay(), Aggregator.aggregate(op, c.field, byDay[it].orEmpty())) }
        val total = formattedValue(c, entity, data, Aggregator.aggregate(op, c.field, days.flatMap { byDay[it].orEmpty() }))
        val metric = if (op == null || op == "count") null else c.field?.let { entity.field(it)?.name }
        return WidgetSnapshot.Trend(metric, total, points)
    }

    /** 集計値 → stat と同じ書式(count は件数・currency の field は ¥)。 */
    private fun formattedValue(c: Component, entity: Entity, data: LeafData, value: Double): String {
        val op = c.aggregate?.wire
        val field = c.field?.let(entity::field)
        val format = c.format?.wire ?: if (op == null || op == "count") "plain" else if (field?.type == FieldType.CURRENCY) "currency" else null
        return Aggregator.format(value, format, if (op == null || op == "count") data.labels.countUnit else field?.unit)
    }

    private fun createdAt(r: Record): Instant? = FieldCoding.date(r[RecordValue.CREATED_AT])
}
