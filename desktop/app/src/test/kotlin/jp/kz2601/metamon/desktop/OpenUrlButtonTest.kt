package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.dsl.App
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.logic.OpenApp
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** openUrlButton の開く先(Android の OpenUrlTarget と同じ分岐)と、候補を順に試す開き方。 */
class OpenUrlButtonTest {
    private fun button(url: String? = null, app: App? = null, to: String? = null) =
        Component(type = ComponentType.OPEN_URL_BUTTON, url = url, app = app, to = to)

    @Test
    fun urlFollowsTheSamePolicyAsLink() {
        assertEquals(OpenUrlTarget.Url("tel:0761234567"), OpenUrlTarget.resolve(button("tel:0761234567"), null))
        assertEquals(OpenUrlTarget.Invalid, OpenUrlTarget.resolve(button("http://example.com"), null))
        assertEquals(OpenUrlTarget.Invalid, OpenUrlTarget.resolve(button("javascript:alert(1)"), null))
        assertEquals(OpenUrlTarget.Invalid, OpenUrlTarget.resolve(button(), null))
    }

    @Test
    fun appWinsOverUrlAndUnknownAppNeverOpens() {
        val x = OpenUrlTarget.resolve(button("https://example.com", App.X, "@tailorit"), null)
        assertEquals(OpenUrlTarget.App(OpenApp.X, listOf("https://x.com/tailorit")), x)
        assertEquals(OpenUrlTarget.Invalid, OpenUrlTarget.resolve(button("https://example.com", App.UNKNOWN), null))
        assertEquals(OpenUrlTarget.App(OpenApp.PHONE, emptyList()), OpenUrlTarget.resolve(button(app = App.PHONE), null))
        assertEquals("renderer_open_line", OpenApp.LINE.labelKey())
    }

    @Test
    fun openFirstTriesNextCandidateOnlyWhenOpeningFails() {
        val s = DesktopSession(Examples.load("shopping"))
        val tried = mutableListOf<String>()
        s.host = ActionHost { a, _, _ -> a.url!!.also { tried += it }.takeIf { it.startsWith("slack:") }?.let { "開けない" } }
        s.openFirst(listOf("slack://open", "https://slack.com/", "https://never"), ActionContext(s.top, null))
        assertEquals(listOf("slack://open", "https://slack.com/"), tried)
        assertNull(s.message)
        // 1 回押して effects は 1 つ(開けた候補だけ)
        assertEquals(1, s.effects.size)
        assertTrue(s.effects.single().toString().contains("https://slack.com/"))
    }

    @Test
    fun openFirstKeepsOnlyTheLastCandidateWhenAllFail() {
        val s = DesktopSession(Examples.load("shopping"))
        s.host = ActionHost { _, _, _ -> "開けない" }
        s.openFirst(listOf("slack://open", "https://slack.com/"), ActionContext(s.top, null))
        assertEquals("開けない", s.message)
        assertEquals(1, s.effects.size)
        assertTrue(s.effects.single().toString().contains("https://slack.com/"))
    }
}
