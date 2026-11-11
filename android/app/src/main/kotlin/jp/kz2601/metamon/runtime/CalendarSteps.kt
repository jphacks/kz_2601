package jp.kz2601.metamon.runtime

import jp.kz2601.metamon.R
import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.logic.CalendarEventDraft
import jp.kz2601.metamon.dsl.logic.CalendarEventPlan
import jp.kz2601.metamon.dsl.logic.request
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.renderer.ActionContext
import java.time.ZoneId

/**
 * `addToCalendar` の前段(iOS / Mac の `CalendarActionSteps.draft`)。どの記録を・どんな予定にするか。
 * 端末は OS の「新規イベント」画面(`Intent.ACTION_INSERT`)に [InsertExtras] を埋めて開き、ユーザーが保存する(権限は要らない)。
 */
object CalendarSteps {
    sealed interface Draft {
        data class Ready(val draft: CalendarEventDraft) : Draft

        /** 予定にしない。toast に出す一言 */
        data class Invalid(val message: UiText) : Draft
    }

    val NO_RECORD = UiText.Res(R.string.host_calendar_no_record)

    /** フォーム画面なら入力中の値(未保存でも)、list の行タップならその行から予定を作る。 */
    fun draft(action: Action, ctx: ActionContext, runtime: AppRuntime): Draft {
        val spec = action.calendarEvent ?: return Draft.Invalid(NO_RECORD)
        val entityId = action.entity ?: ctx.form?.entity?.id ?: return Draft.Invalid(NO_RECORD)
        val entity = runtime.entityModel(entityId) ?: return Draft.Invalid(NO_RECORD)
        val form = ctx.form
        val data: Map<String, JSONValue> = when {
            form != null && form.entity.id == entityId -> form.payload
            ctx.recordId != null -> runtime.records(entityId).firstOrNull { it.id == ctx.recordId }?.data
            else -> null
        } ?: return Draft.Invalid(NO_RECORD)
        return when (val r = CalendarEventPlan.build(spec.request, entity, data)) {
            is CalendarEventPlan.Result.Success -> Draft.Ready(r.draft)
            is CalendarEventPlan.Result.NoStart -> Draft.Invalid(UiText.Res(R.string.host_calendar_no_start, r.fieldName))
        }
    }

    /**
     * `ACTION_INSERT` に載せる値(CalendarContract の EXTRA_EVENT_BEGIN_TIME など)。epoch ミリ秒。
     * 終日は端末の時刻の 0 時から、終わりは最後の日の翌日 0 時(Android の終日は終わりを含まない)。
     * 通知(alertMinutes)はこの画面に渡す口が無いので載せない(ユーザーが画面で選ぶ)。
     */
    data class InsertExtras(
        val title: String,
        val beginMillis: Long,
        val endMillis: Long,
        val allDay: Boolean,
        val location: String?,
        val description: String?,
    )

    fun insertExtras(draft: CalendarEventDraft, zone: ZoneId = ZoneId.systemDefault()): InsertExtras {
        if (!draft.isAllDay) {
            return InsertExtras(draft.title, draft.start.toEpochMilli(), draft.end.toEpochMilli(), false, draft.location, draft.notes)
        }
        val first = draft.start.atZone(zone).toLocalDate()
        val last = maxOf(first, draft.end.atZone(zone).toLocalDate())
        return InsertExtras(
            title = draft.title,
            beginMillis = first.atStartOfDay(zone).toInstant().toEpochMilli(),
            endMillis = last.plusDays(1).atStartOfDay(zone).toInstant().toEpochMilli(),
            allDay = true,
            location = draft.location,
            description = draft.notes,
        )
    }
}
