package jp.kz2601.metamon.desktop.voice

import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.StringTable
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/** 頼み方のコツ: 文言のキーが日本語・英語で全部引ける・デスクトップで使えない機能の例を出さない(#2972)。 */
class PromptTipsTest {
    private val table = StringTable.fromClasspath()

    @Test
    fun everyKeyResolvesInJapaneseAndEnglish() {
        for (c in PromptTips.all) {
            for (lang in Lang.entries) {
                assertTrue("${c.titleKey} ($lang)", table.hasString(lang, c.titleKey))
                for (t in c.tips) assertTrue("${t.textKey} ($lang)", table.hasString(lang, t.textKey))
            }
        }
    }

    @Test
    fun unavailableFeaturesAreNotOffered() {
        val shown = PromptTips.categories().flatMap { it.tips }
        val ids = shown.map { it.id }.toSet()
        for (id in listOf("photo_camera", "share_nearby", "stats_steps", "map_route")) assertFalse(id, id in ids)
        assertTrue(shown.none { it.requires.any { f -> f !in PromptTips.available } })
        assertTrue("share_family" in ids)
    }

    @Test
    fun idsAreUniqueAndAllFeaturesKeepEveryCategory() {
        val ids = PromptTips.all.flatMap { it.tips }.map { it.id }
        assertEquals(ids.size, ids.toSet().size)
        assertEquals(PromptTips.all.map { it.id }, PromptTips.categories(PromptTips.Feature.entries.toSet()).map { it.id })
    }
}
