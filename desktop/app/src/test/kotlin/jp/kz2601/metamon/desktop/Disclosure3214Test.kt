package jp.kz2601.metamon.desktop

import androidx.compose.ui.input.key.Key
import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/** #3214: disclosure は expanded の既定を見て畳み、見出しで開け閉めする。 */
class Disclosure3214Test {
    private fun disclosure(expanded: Boolean?, id: String? = "more", title: String? = "Details") =
        Component(type = ComponentType.DISCLOSURE, id = id, title = title, expanded = expanded)

    @Test
    fun expandedDefaultIsUsedAndMissingMeansCollapsed() {
        val memory = DisclosureMemory()
        assertTrue(memory.isExpanded("s#a", disclosure(true)))
        assertFalse(memory.isExpanded("s#b", disclosure(false)))
        assertFalse(memory.isExpanded("s#c", disclosure(null)))
    }

    @Test
    fun toggleFlipsAndIsRememberedPerKey() {
        val memory = DisclosureMemory()
        val open = disclosure(true)
        val key = Disclosure.key("home", open)
        memory.toggle(key, open)
        assertFalse(memory.isExpanded(key, open))
        memory.toggle(key, open)
        assertTrue(memory.isExpanded(key, open))
        // 別の画面の同じ id は別に覚える
        val closed = disclosure(null)
        memory.toggle(Disclosure.key("home", closed), closed)
        assertFalse(memory.isExpanded(Disclosure.key("other", closed), closed))
        // 新しいウィンドウ(新しい覚え)は既定に戻る
        assertTrue(DisclosureMemory().isExpanded(key, open))
    }

    @Test
    fun keyUsesIdThenTitle() {
        assertEquals("home#more", Disclosure.key("home", disclosure(null)))
        assertEquals("home#Details", Disclosure.key("home", disclosure(null, id = null)))
    }

    @Test
    fun enterAndSpaceToggle() {
        assertTrue(Disclosure.isToggleKey(Key.Enter))
        assertTrue(Disclosure.isToggleKey(Key.NumPadEnter))
        assertTrue(Disclosure.isToggleKey(Key.Spacebar))
        assertFalse(Disclosure.isToggleKey(Key.Tab))
    }

    @Test
    fun spokenStateAndDefaultTitle() = Strings.withLang(Lang.JA) {
        assertEquals("開いています", Disclosure.stateText(true))
        assertEquals("閉じています", Disclosure.stateText(false))
        assertEquals("詳細", Disclosure.title(disclosure(null, title = null)))
    }
}
