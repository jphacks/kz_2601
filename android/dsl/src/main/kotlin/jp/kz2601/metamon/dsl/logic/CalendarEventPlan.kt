package jp.kz2601.metamon.dsl.logic

import jp.kz2601.metamon.dsl.expr.SwiftNumbers
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.dsl.records.JSONValue
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.doubleOrNull
import java.time.Instant

/** `addToCalendar.calendarEvent` の中身(iOS の `CalendarEventSpec`)。field id はフォームの entity のもの。 */
data class CalendarEventRequest(
    /** date なら終日、datetime ならその時刻から */
    val startField: String,
    val endField: String? = null,
    /** 終わりが無い時の長さ(分)。null は 60 */
    val durationMinutes: Int? = null,
    /** null は entity.titleField → 最初の文字の field */
    val titleField: String? = null,
    val locationField: String? = null,
    val notesField: String? = null,
    /** 始まりの N 分前に通知(0 は始まりの時刻) */
    val alertMinutes: Int? = null,
) {
    companion object {
        /** startField が無ければ null。他のキーは型が合わなければ無視。 */
        fun from(e: JsonElement): CalendarEventRequest? {
            val o = e as? JsonObject ?: return null
            fun str(k: String) = (o[k] as? JsonPrimitive)?.takeIf { it.isString }?.content
            fun int(k: String) = (o[k] as? JsonPrimitive)?.takeIf { !it.isString }?.doubleOrNull?.takeIf { it == Math.rint(it) }?.toInt()
            return CalendarEventRequest(
                startField = str("startField") ?: return null,
                endField = str("endField"),
                durationMinutes = int("durationMinutes"),
                titleField = str("titleField"),
                locationField = str("locationField"),
                notesField = str("notesField"),
                alertMinutes = int("alertMinutes"),
            )
        }
    }
}

/** OS の「新規イベント」画面に埋める中身(CalendarContract を知らない値だけ)。iOS の `CalendarEventDraft`。 */
data class CalendarEventDraft(
    val title: String,
    val start: Instant,
    val end: Instant,
    val isAllDay: Boolean,
    val location: String?,
    val notes: String?,
    /** 始まりからの通知の位置(秒・負 = 前)。null は通知なし */
    val alertOffsetSeconds: Long?,
)

/** レコードの値 → 予定。date の field は終日、datetime はその時刻から(終わりが無ければ durationMinutes・既定 60 分)。 */
object CalendarEventPlan {
    sealed interface Result {
        data class Success(val draft: CalendarEventDraft) : Result
        /** 始まりの field が空・読めない。値はその field の名前 */
        data class NoStart(val fieldName: String) : Result
    }

    const val DEFAULT_MINUTES = 60

    fun build(spec: CalendarEventRequest, entity: Entity, data: Map<String, JSONValue>): Result {
        val startField = entity.field(spec.startField)
        val start = data[spec.startField]?.let { FieldCoding.date(it) } ?: return Result.NoStart(startField?.name ?: spec.startField)
        val isAllDay = startField?.type == FieldType.DATE
        var end = spec.endField?.let { data[it] }?.let { FieldCoding.date(it) }
        if (end != null && end < start) end = null // 終わりが始まりより前は無視(入力の途中など)
        val fallback = if (isAllDay) start else start.plusSeconds((spec.durationMinutes ?: DEFAULT_MINUTES) * 60L)
        val titleId = spec.titleField ?: entity.displayTitleField
        val title = titleId?.let { text(data[it]) } ?: entity.name
        return Result.Success(
            CalendarEventDraft(
                title = title,
                start = start,
                end = end ?: fallback,
                isAllDay = isAllDay,
                location = spec.locationField?.let { text(data[it]) },
                notes = spec.notesField?.let { text(data[it]) },
                alertOffsetSeconds = spec.alertMinutes?.let { -(maxOf(it, 0) * 60L) },
            ),
        )
    }

    /** 文字の値(空白だけは無し)。数値は整数ならそのまま(「第 3 回」の 3 など)。 */
    internal fun text(value: JSONValue?): String? = when (value) {
        is JSONValue.Str -> value.value.trimWhitespace().ifEmpty { null }
        is JSONValue.Num -> {
            val n = value.value
            if (n == SwiftNumbers.rounded(n)) n.toLong().toString() else SwiftNumbers.description(n)
        }
        else -> null
    }
}
