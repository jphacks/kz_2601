package jp.kz2601.metamon.desktop.store

import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.StringTable
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.dsl.records.entityModels
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.Instant

/** 保存期間の確認(#482・#2977): 確認の状態を書く・確認するまで消さない・「あとで」は聞き直さない・設定の一覧。 */
class RetentionPromptTest {
    private val now = Instant.parse("2026-11-13T00:00:00Z")

    private val dsl = AppDSL.decode(
        """
        {"schemaVersion":1,"app":{"name":"記録"},"initialScreen":"home",
         "entities":[
          {"id":"logs","name":"日記","retentionDays":30,"fields":[{"id":"name","name":"名前","type":"string"}]},
          {"id":"posts","name":"投稿","scope":"shared","retentionDays":7,"fields":[{"id":"name","name":"名前","type":"string"}]},
          {"id":"memo","name":"メモ","fields":[{"id":"name","name":"名前","type":"string"}]}
         ],
         "screens":[{"id":"home","title":"ホーム","components":[]}]}
        """.trimIndent(),
    )
    private val models = dsl.entityModels()

    private fun runner() = RetentionRunner(KeyValueRetentionConsent(JsonKeyValueStore(null)), now = { now })

    @Test
    fun asksOnlyForEntitiesWithRetentionAndNotConfirmedYet() {
        val prompt = RetentionPrompts.pending("app", runner(), models)!!
        assertEquals(listOf("logs", "posts"), prompt.items.map { it.id })
    }

    @Test
    fun nothingIsPurgedUntilConfirmedThenOnlyThatEntity() {
        val r = runner()
        val old = Record("a", mapOf("name" to JSONValue.Str("a")), createdAt = "2026-09-01T00:00:00Z")
        val rows = mapOf("logs" to listOf(old))
        assertTrue("確認前は消さない", r.expired("app", models, rows, "me").isEmpty())
        val prompt = RetentionPrompts.pending("app", r, models)!!
        RetentionPrompts.confirm(r, prompt)
        assertEquals(30, r.consentedDays("app", "logs"))
        assertEquals(7, r.consentedDays("app", "posts"))
        assertNull("保存期間の無い entity には印を書かない", r.consentedDays("app", "memo"))
        assertEquals(mapOf("logs" to listOf("a")), r.expired("app", models, rows, "me"))
        assertNull("確認したら、もう聞かない", RetentionPrompts.pending("app", r, models))
    }

    @Test
    fun laterIsNotAskedAgainAndDoesNotWriteAMark() {
        val r = runner()
        val prompt = RetentionPrompts.pending("later-app", r, models)!!
        RetentionPrompts.defer(prompt)
        assertNull(RetentionPrompts.pending("later-app", r, models))
        assertNull(r.consentedDays("later-app", "logs"))
        assertNotNull("別のアプリは聞く", RetentionPrompts.pending("other-app", r, models))
    }

    @Test
    fun shorterPeriodNeedsConfirmingAgain() {
        val r = runner()
        RetentionPrompts.confirm(r, RetentionPrompts.pending("short-app", r, models)!!)
        val shorter = AppDSL.decode(
            """
            {"schemaVersion":1,"app":{"name":"記録"},"initialScreen":"home",
             "entities":[{"id":"logs","name":"日記","retentionDays":14,"fields":[{"id":"name","name":"名前","type":"string"}]}],
             "screens":[{"id":"home","title":"ホーム","components":[]}]}
            """.trimIndent(),
        ).entityModels()
        assertEquals(listOf("logs"), RetentionPrompts.pending("short-app", r, shorter)!!.items.map { it.id })
    }

    @Test
    fun messageNamesEntityAndPeriodAndWarnsAboutShared() {
        val prompt = RetentionPrompts.pending("msg-app", runner(), models)!!
        assertEquals(
            "「日記」は30日、「投稿」は1週間たった記録を自動で消します。消えた記録は元に戻せません。共有の記録は自分が追加したものだけが消えます。",
            prompt.message(Lang.JA),
        )
        assertTrue(prompt.message(Lang.EN).contains("30 days"))
        assertTrue(prompt.message(Lang.EN).contains("1 week"))
    }

    @Test
    fun reviewRowsShowAppEntityDaysAndConfirmedState() {
        val r = runner()
        r.confirm("rows-app", models.first { it.id == "logs" })
        val rows = RetentionPrompts.rows("rows-app", "記録", r, models)
        assertEquals(listOf("logs", "posts"), rows.map { it.entityId })
        assertEquals(listOf(true, false), rows.map { it.confirmed })
        assertEquals("記録、日記、30日で消す、確認済み", rows[0].description(Lang.JA))
        assertEquals("記録、投稿、1週間で消す、未確認", rows[1].description(Lang.JA))
        assertFalse(rows.any { it.entityId == "memo" })
    }

    @Test
    fun stringsResolveInBothLanguages() {
        val t = StringTable.fromClasspath()
        val keys = listOf(
            "retention_title", "retention_message", "retention_message_item", "retention_message_separator", "retention_settings_footer_shared",
            "retention_confirm", "retention_later", "retention_settings_header", "retention_settings_footer", "retention_settings_period",
            "retention_settings_confirm", "desktop_retention_confirmed", "desktop_retention_unconfirmed", "desktop_retention_review",
        )
        for (lang in Lang.entries) for (k in keys) assertTrue("$k ($lang)", t.hasString(lang, k))
    }
}
