package jp.kz2601.metamon.services.privacy

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/** iOS の PublicPersonalDataTests(#253): 確認を出すか・ダイアログの文と指示文。 */
class PublicPersonalDataTest {
    private val fields = listOf(
        PublicPersonalField("reservations", "予約", "guestName", "お名前", "name"),
        PublicPersonalField("reservations", "予約", "phone", "電話番号", "phone"),
        PublicPersonalField("members", "会員", "address", "住所", "address"),
    )

    @Test
    fun promptTextAndInstructions() {
        val p = PublicDataPrompt.of(fields, 2)!!
        assertEquals("お名前・電話番号・住所", p.fieldNames())
        assertEquals("予約の「お名前」と「電話番号」の欄と、会員の「住所」の欄を消して", p.removeInstruction())
        assertEquals("予約と会員を自分だけの記録にして", p.privateInstruction())
        assertEquals("お名前, 電話番号, 住所", p.fieldNames(english = true))
        assertEquals("""Remove the "お名前" and "電話番号" fields from 予約, and the "住所" field from 会員""", p.removeInstruction(english = true))
        assertEquals("Keep 予約 and 会員 to only me (make it private)", p.privateInstruction(english = true))

        val one = PublicDataPrompt.of(listOf(fields[1]), 1)!!
        assertEquals("予約の「電話番号」の欄を消して", one.removeInstruction())
        assertEquals("予約を自分だけの記録にして", one.privateInstruction())
    }

    @Test
    fun confirmOnlyWhenServerReportsFields() {
        assertNull("旧サーバー(フィールド無し)では出さない", PublicDataPrompt.of(emptyList(), 1))
        assertNull(PublicDataPrompt.fromMeta(emptyList(), 1))
        val raw = listOf(
            Json.parseToJsonElement("""{"entity":"reservations","entityName":"予約","field":"phone","fieldName":"電話番号","kind":"phone"}""") as JsonObject,
            Json.parseToJsonElement("""{"entity":"reservations","field":"guestName"}""") as JsonObject,
            Json.parseToJsonElement("""{"field":"broken"}""") as JsonObject,
        )
        val p = PublicDataPrompt.fromMeta(raw, 3)!!
        assertEquals(3, p.version)
        assertEquals(listOf("phone", "guestName"), p.fields.map { it.field })
        assertEquals(PublicPersonalField("reservations", "reservations", "guestName", "guestName", ""), p.fields[1])
        assertNull("壊れた欄だけなら出さない", PublicDataPrompt.fromMeta(raw.takeLast(1), 3))
    }

    @Test
    fun duplicateNamesShownOnce() {
        val dup = fields.take(1) + PublicPersonalField("reservations", "予約", "guestName2", "お名前", "name")
        val p = PublicDataPrompt.of(dup, 1)!!
        assertEquals("お名前", p.fieldNames())
        assertEquals("予約の「お名前」の欄を消して", p.removeInstruction())
    }
}
