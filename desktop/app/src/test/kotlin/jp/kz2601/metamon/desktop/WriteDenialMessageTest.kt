package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.services.http.ApiError
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** 共有のアプリで書き込みを断られた時の文言: 種類ごとの文言・知らない理由は汎用の文言。 */
class WriteDenialMessageTest {
    private fun server(status: Int, reason: String?, vararg extra: Pair<String, kotlinx.serialization.json.JsonElement>) = ApiError.Server(
        status,
        "forbidden",
        "",
        JsonObject((if (reason != null) mapOf("reason" to JsonPrimitive(reason)) else emptyMap()) + extra),
    )

    private val names = mapOf("price" to "値段", "name" to "名前")

    @Test
    fun eachKindHasItsOwnMessage() {
        fun msg(e: Throwable) = WriteDenialMessage.of(e) { names[it] }
        assertEquals(Strings["desktop_denial_admins_only"], msg(server(403, "admins_only")))
        assertEquals(Strings.format("desktop_denial_admins_only_field", "値段"), msg(server(403, "admins_only", "field" to JsonPrimitive("price"))))
        assertEquals(Strings["desktop_denial_not_member"], msg(server(403, "not_member")))
        assertEquals(Strings["desktop_denial_inviter_left"], msg(server(403, "inviter_left")))
        assertEquals(Strings["desktop_denial_members_full"], msg(server(403, "members_full")))
        assertEquals(Strings["desktop_denial_duplicate"], msg(server(409, "duplicate")))
        assertEquals(
            Strings.format("desktop_denial_duplicate_fields", "名前${Strings["desktop_list_separator"]}値段"),
            msg(server(409, "duplicate", "fields" to JsonArray(listOf(JsonPrimitive("name"), JsonPrimitive("price"))))),
        )
    }

    @Test
    fun unknownReasonIsGenericAndNonDenialIsNull() {
        assertEquals(Strings["desktop_denial_generic"], WriteDenialMessage.of(server(403, "something_new")))
        assertEquals(Strings["desktop_denial_generic"], WriteDenialMessage.of(server(403, null)))
        assertNull(WriteDenialMessage.of(server(500, null)))
        assertNull(WriteDenialMessage.of(IllegalStateException("x")))
    }

    @Test
    fun englishAndDraftSurvives() {
        Strings.withLang(Lang.EN) { assertTrue(WriteDenialMessage.of(server(403, "admins_only"))!!.startsWith("Only admins")) }
        val s = DesktopSession(Examples.load("shopping"), MockStore())
        assertTrue(s.showWriteDenial(server(403, "admins_only")))
        assertEquals(Strings["desktop_denial_admins_only"], s.message)
        s.message = null
        assertEquals(false, s.showWriteDenial(server(500, null)))
        assertNull(s.message)
    }
}
