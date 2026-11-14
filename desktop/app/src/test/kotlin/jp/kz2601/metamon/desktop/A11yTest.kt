package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/** 読み上げの説明(絵で描く部品を言葉にする・#1759)。 */
class A11yTest {
    private val data = LeafData(emptyList(), { emptyList() })

    @Test
    fun graphicLeavesGetADescriptionAndTextLeavesDoNot() {
        assertEquals("レシート", A11y.describe(Component(type = ComponentType.IMAGE, label = "レシート"), data))
        assertEquals("画像", A11y.describe(Component(type = ComponentType.IMAGE), data))
        assertEquals("Image", Strings.withLang(Lang.EN) { A11y.describe(Component(type = ComponentType.IMAGE), data) })
        assertEquals("Avatar", Strings.withLang(Lang.EN) { A11y.describe(Component(type = ComponentType.AVATAR), data) })
        assertEquals("グラフ: まだデータがありません", A11y.describe(Component(type = ComponentType.CHART), data))
        assertNull(A11y.describe(Component(type = ComponentType.TEXT, text = "こんにちは"), data))
        assertNull(A11y.describe(Component(type = ComponentType.BUTTON, label = "保存"), data))
    }
}
