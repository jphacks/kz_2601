package jp.kz2601.metamon.desktop.privacy

import jp.kz2601.metamon.desktop.i18n.Lang
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/** 生成直後の公開確認: meta が無い時は出さない / 英語の指示文 / 公開のままは何も送らない。 */
class PublicDataPromptDesktopTest {
    private fun field(entity: String, entityName: String, field: String, name: String, kind: String): JsonObject =
        Json.parseToJsonElement("""{"entity":"$entity","entityName":"$entityName","field":"$field","fieldName":"$name","kind":"$kind"}""").jsonObject

    private val meta = listOf(
        field("booking", "予約", "name", "お名前", "name"),
        field("booking", "予約", "tel", "電話番号", "phone"),
    )

    @Test
    fun noMetaMeansNoPrompt() {
        assertNull(PublicData.prompt(emptyList(), 1))
        assertNull("壊れた欄だけでも出さない", PublicData.prompt(listOf(Json.parseToJsonElement("""{"kind":"name"}""").jsonObject), 1))
    }

    @Test
    fun japaneseInstructions() {
        val p = PublicData.prompt(meta, 3)!!
        assertEquals("予約の「お名前」と「電話番号」の欄を消して", PublicData.instruction(p, PublicDataChoice.REMOVE_FIELDS, Lang.JA))
        assertEquals("予約を自分だけの記録にして", PublicData.instruction(p, PublicDataChoice.MAKE_PRIVATE, Lang.JA))
    }

    @Test
    fun englishInstructions() {
        val p = PublicData.prompt(meta, 3)!!
        assertEquals("Remove the \"お名前\" and \"電話番号\" fields from 予約", PublicData.instruction(p, PublicDataChoice.REMOVE_FIELDS, Lang.EN))
        assertEquals("Keep 予約 to only me (make it private)", PublicData.instruction(p, PublicDataChoice.MAKE_PRIVATE, Lang.EN))
    }

    @Test
    fun keepingPublicSendsNothing() {
        assertNull(PublicData.instruction(PublicData.prompt(meta, 3)!!, PublicDataChoice.KEEP, Lang.JA))
    }
}
