package jp.kz2601.metamon.renderer

import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.Size
import jp.kz2601.metamon.dsl.TextHighlight
import jp.kz2601.metamon.dsl.TextHighlightStyle
import jp.kz2601.metamon.renderer.components.ExternalUrlPolicy
import jp.kz2601.metamon.renderer.components.InlineMarkdown
import jp.kz2601.metamon.renderer.components.InlineMarkdown.Span
import jp.kz2601.metamon.renderer.components.PhotoValue
import jp.kz2601.metamon.renderer.components.avatarInitials
import jp.kz2601.metamon.renderer.components.highlighted
import jp.kz2601.metamon.renderer.components.spacerHeight
import jp.kz2601.metamon.renderer.conformance.RenderTree
import jp.kz2601.metamon.renderer.registry.ComponentRegistry
import jp.kz2601.metamon.renderer.theme.SymbolIcons
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.AppMeta
import jp.kz2601.metamon.dsl.Screen
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** 表示系の Leaf 11 種(#857)の解決の関数と、描画の木に出る値。 */
class DisplayLeavesTest {
    private val display = listOf(
        ComponentType.TEXT, ComponentType.LABEL, ComponentType.ICON, ComponentType.MARKDOWN, ComponentType.BADGE,
        ComponentType.AVATAR, ComponentType.IMAGE, ComponentType.DIVIDER, ComponentType.SPACER, ComponentType.LINK,
        ComponentType.EMPTY_STATE,
    )

    @Test
    fun all11AreRegistered() {
        val registered = ComponentRegistry.standard.registeredTypes
        display.forEach { assertTrue("${it.wire} が未登録", it.wire in registered) }
    }

    @Test
    fun markdownInlineRange() {
        assertEquals(
            listOf(Span("a "), Span("b", bold = true), Span(" "), Span("i", italic = true), Span(" "), Span("c", code = true)),
            InlineMarkdown.parse("a **b** *i* `c`"),
        )
        assertEquals(listOf(Span("x", bold = true), Span(" "), Span("y", italic = true), Span(" "), Span("z", strike = true)), InlineMarkdown.parse("__x__ _y_ ~~z~~"))
        assertEquals(listOf(Span("公式", url = "https://jphacks.com"), Span(" へ")), InlineMarkdown.parse("[公式](https://jphacks.com) へ"))
        assertEquals(listOf(Span("**太字", bold = true)), InlineMarkdown.parse("**\\*\\*太字**"))
        assertEquals(listOf(Span("https://a.b", url = "https://a.b")), InlineMarkdown.parse("<https://a.b>"))
        assertEquals("閉じない記号は文字のまま", "**開いたまま と *これも", InlineMarkdown.plain("**開いたまま と *これも"))
        assertEquals("語の途中の _ は斜体にしない", "snake_case_name", InlineMarkdown.plain("snake_case_name"))
        assertEquals("ブロックの記法・改行はそのまま", "# 見出し\n- 項目", InlineMarkdown.plain("# 見出し\n- 項目"))
        assertEquals(listOf(Span("太字の中の", bold = true), Span("斜体", bold = true, italic = true)), InlineMarkdown.parse("**太字の中の*斜体***"))
    }

    @Test
    fun urlPolicyMatchesIos() {
        assertNotNull(ExternalUrlPolicy.openable("https://jphacks.com"))
        assertNotNull(ExternalUrlPolicy.openable("tel:0120000000"))
        assertNotNull(ExternalUrlPolicy.openable("mailto:a@example.com"))
        assertNull("http は開かない", ExternalUrlPolicy.openable("http://example.com"))
        assertNull(ExternalUrlPolicy.openable("javascript:alert(1)"))
        assertNull(ExternalUrlPolicy.openable("intent://x#Intent;end"))
        assertNull(ExternalUrlPolicy.openable(null))
        assertNull(ExternalUrlPolicy.openable("not a url"))
    }

    @Test
    fun avatarInitialsMatchIos() {
        assertEquals("山太", avatarInitials("山田 太郎"))
        assertEquals("山", avatarInitials("山田"))
        assertEquals("KO", avatarInitials("kota"))
        assertEquals("kt", avatarInitials("kota takaoka"))
        assertEquals("", avatarInitials("  "))
        assertEquals("", avatarInitials(null))
    }

    @Test
    fun photoValues() {
        assertTrue(PhotoValue.hasPhoto("data:image/jpeg;base64,/9j/4AAQ"))
        assertTrue(PhotoValue.hasPhoto("https://example.com/a.jpg"))
        assertFalse(PhotoValue.hasPhoto("data:image/jpeg;base64,"))
        assertFalse(PhotoValue.hasPhoto(""))
        assertFalse(PhotoValue.hasPhoto(null))
    }

    @Test
    fun iconsAndSizes() {
        assertNotNull(SymbolIcons.vector("checkmark"))
        assertNotNull("修飾を外して引く", SymbolIcons.vector("heart.fill"))
        assertNotNull(SymbolIcons.vector("person.crop.circle.fill"))
        assertNull("無い名前は既定の印", SymbolIcons.vector("refrigerator"))
        assertEquals(8.dp, Component(ComponentType.SPACER, size = Size.SMALL).spacerHeight)
        assertEquals(0.dp, Component(ComponentType.SPACER, size = Size.FLEXIBLE).spacerHeight)
    }

    @Test
    fun highlightKeepsTextAndMarksWords() {
        val c = Component(ComponentType.TEXT, text = "期限は明日", highlight = TextHighlight(words = listOf("期限")))
        val s = highlighted(c, null)
        assertEquals("文そのものは変えない(読み上げはそのまま)", "期限は明日", s.text)
        assertEquals(1, s.spanStyles.size)
        assertEquals(0 until 2, s.spanStyles[0].start until s.spanStyles[0].end)
        assertEquals(FontWeight.Bold, s.spanStyles[0].item.fontWeight)
        val bold = highlighted(c.copy(highlight = TextHighlight(words = listOf("明日"), style = TextHighlightStyle.BOLD)), null)
        assertEquals("bold は背景を付けない", androidx.compose.ui.graphics.Color.Unspecified, bold.spanStyles[0].item.background)
    }

    @Test
    fun renderTreeProps() {
        val dsl = AppDSL(
            schemaVersion = 1,
            app = AppMeta(),
            initialScreen = "s",
            screens = listOf(
                Screen(
                    id = "s",
                    title = "S",
                    components = listOf(
                        Component(ComponentType.LINK, id = "l1", text = "公式", url = "https://jphacks.com"),
                        Component(ComponentType.LINK, id = "l2", url = "https://a.b/c"),
                        Component(ComponentType.EMPTY_STATE, id = "e", title = "まだありません", message = "追加してね", icon = "tray"),
                        Component(ComponentType.IMAGE, id = "i", field = "photo"),
                        Component(ComponentType.AVATAR, id = "a", name = "山田 太郎"),
                    ),
                ),
            ),
        )
        val tree = RenderTree(dsl, ComponentRegistry.standard.registeredTypes)
        val nodes = dsl.screens[0].components.map { tree.node(it, null) }
        fun props(i: Int) = nodes[i]["props"]?.jsonObject
        assertEquals(JsonObject(mapOf("text" to JsonPrimitive("公式"), "url" to JsonPrimitive("https://jphacks.com"), "opens" to JsonPrimitive(true))), props(0))
        assertEquals(JsonPrimitive("https://a.b/c"), props(1)!!["text"])
        assertEquals(JsonObject(mapOf("title" to JsonPrimitive("まだありません"), "message" to JsonPrimitive("追加してね"))), props(2))
        assertEquals(JsonObject(mapOf("field" to JsonPrimitive("photo"), "hasImage" to JsonPrimitive(false))), props(3))
        assertNull("avatar は見た目のキーだけなので props なし", props(4))
        nodes.forEach { assertEquals(JsonPrimitive("rendered"), it["state"]) }
    }
}
