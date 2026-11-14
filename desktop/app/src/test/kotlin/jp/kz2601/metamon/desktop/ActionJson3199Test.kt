package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionFormat
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.Presentation
import kotlinx.serialization.json.JsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Test

/** DesktopSession.actionJson(#3199): effects の `request` に入る操作の鍵(iOS の適合テストの形)を ActionType ごとに表で。 */
class ActionJson3199Test {
    private val allKeys = setOf("type", "screen", "presentation", "prefillRef", "entity", "url", "confirmMessage", "format", "notificationId")

    private fun full(type: ActionType) = Action(
        type = type, screen = "s", presentation = Presentation.SHEET, prefillRef = "p", entity = "e", url = "https://example.com",
        confirmMessage = "m", format = ActionFormat.CSV, notificationId = "n",
    )

    @Test
    fun actionJsonKeysPerType() {
        // type ごとに、実際に付く項目だけを持たせた Action → 出る鍵
        val table: Map<ActionType, Pair<Action, Set<String>>> = mapOf(
            ActionType.NAVIGATE to (Action(type = ActionType.NAVIGATE, screen = "form", presentation = Presentation.PUSH, prefillRef = "row") to setOf("type", "screen", "presentation", "prefillRef")),
            ActionType.BACK to (Action(type = ActionType.BACK) to setOf("type")),
            ActionType.SAVE_RECORD to (Action(type = ActionType.SAVE_RECORD, entity = "e") to setOf("type", "entity")),
            ActionType.DELETE_RECORD to (Action(type = ActionType.DELETE_RECORD, entity = "e", confirmMessage = "消す?") to setOf("type", "entity", "confirmMessage")),
            ActionType.FOLLOW to (Action(type = ActionType.FOLLOW, entity = "e") to setOf("type", "entity")),
            ActionType.UNFOLLOW to (Action(type = ActionType.UNFOLLOW, entity = "e") to setOf("type", "entity")),
            ActionType.OPEN_VOICE to (Action(type = ActionType.OPEN_VOICE) to setOf("type")),
            ActionType.OPEN_URL to (Action(type = ActionType.OPEN_URL, url = "https://example.com") to setOf("type", "url")),
            ActionType.RESET_FORM to (Action(type = ActionType.RESET_FORM) to setOf("type")),
            ActionType.SCHEDULE_NOTIFICATION to (Action(type = ActionType.SCHEDULE_NOTIFICATION, notificationId = "n") to setOf("type", "notificationId")),
            ActionType.CANCEL_NOTIFICATION to (Action(type = ActionType.CANCEL_NOTIFICATION, notificationId = "n") to setOf("type", "notificationId")),
            ActionType.EXPORT to (Action(type = ActionType.EXPORT, entity = "e", format = ActionFormat.PDF) to setOf("type", "entity", "format")),
            ActionType.START_ACTIVITY to (Action(type = ActionType.START_ACTIVITY) to setOf("type")),
            ActionType.END_ACTIVITY to (Action(type = ActionType.END_ACTIVITY) to setOf("type")),
            ActionType.ADD_TO_CALENDAR to (Action(type = ActionType.ADD_TO_CALENDAR) to setOf("type")),
            ActionType.LOOKUP_BOOK to (Action(type = ActionType.LOOKUP_BOOK) to setOf("type")),
        )
        val known = ActionType.entries.filter { it != ActionType.UNKNOWN }
        assertEquals("全部の ActionType を表に入れる(増えたらここも足す)", known.toSet(), table.keys)
        for ((type, case) in table) {
            val json = DesktopSession.actionJson(case.first)
            assertEquals("$type の鍵", case.second, json.keys)
            assertEquals("$type の type は wire", JsonPrimitive(type.wire), json["type"])
        }
    }

    @Test
    fun everyTypeEmitsAllKeysWhenAllSet() {
        for (type in ActionType.entries) {
            val json = DesktopSession.actionJson(full(type))
            assertEquals("$type", allKeys, json.keys)
            assertEquals(JsonPrimitive("sheet"), json["presentation"])
            assertEquals(JsonPrimitive("csv"), json["format"])
            assertEquals(JsonPrimitive("https://example.com"), json["url"])
        }
    }

    @Test
    fun unknownTypeKeepsRawType() {
        val json = DesktopSession.actionJson(Action(type = ActionType.UNKNOWN, rawType = "futureAction"))
        assertEquals(setOf("type"), json.keys)
        assertEquals(JsonPrimitive("futureAction"), json["type"])
    }
}
