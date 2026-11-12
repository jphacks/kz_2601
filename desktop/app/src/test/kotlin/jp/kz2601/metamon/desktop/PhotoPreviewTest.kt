package jp.kz2601.metamon.desktop

import androidx.compose.ui.input.key.Key
import jp.kz2601.metamon.desktop.io.DragOut
import jp.kz2601.metamon.dsl.expr.Expr
import jp.kz2601.metamon.dsl.records.Field
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.dsl.records.JSONValue
import kotlinx.serialization.json.JsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File
import java.util.Base64

/** 写真を大きく見る(Mac の Quick Look・#1850 のデスクトップ版): 見せる物の集め方・開く / 閉じる・キー。 */
class PhotoPreviewTest {
    private val png1x1 = Base64.getDecoder().decode("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==")
    private val a = "data:image/png;base64," + Base64.getEncoder().encodeToString(png1x1)
    private val b = "data:image/jpeg;base64,/9j/AAAA"

    private fun photo(id: String, name: String = id, hidden: Boolean = false) = Field(id, name, FieldType.PHOTO, hidden = hidden)

    // MARK: - 見せる物

    @Test
    fun photosFollowFieldOrderAndSkipHiddenComputedAndOtherTypes() {
        val fields = listOf(
            photo("after", "後"),
            Field("title", "題", FieldType.STRING),
            photo("secret", "非公開", hidden = true),
            Field("calc", "算出", FieldType.PHOTO, computed = Expr.from(JsonPrimitive("x"))),
            photo("before", "前"),
            photo("empty", "空"),
        )
        val values = mapOf(
            "before" to JSONValue.Str(a),
            "after" to JSONValue.Str(b),
            "title" to JSONValue.Str(a),
            "secret" to JSONValue.Str(a),
            "calc" to JSONValue.Str(a),
        )
        assertEquals(listOf(PreviewPhoto(b, "後"), PreviewPhoto(a, "前")), PhotoPreviewSources.photos(values, fields))
    }

    @Test
    fun onlyDataUrlsAndLocalImageFilesAreShown() {
        assertTrue(PhotoPreviewSources.accepts(a))
        assertTrue(PhotoPreviewSources.accepts(File("/tmp/tailorit-photos/p.JPG").toURI().toString()))
        // https は出さない(Mac と同じ)・画像でないファイル・data URL の他の種類・空
        assertFalse(PhotoPreviewSources.accepts("https://example.com/a.png"))
        assertFalse(PhotoPreviewSources.accepts("http://example.com/a.png"))
        assertFalse(PhotoPreviewSources.accepts(File("/etc/passwd").toURI().toString()))
        assertFalse(PhotoPreviewSources.accepts(File("/tmp/a.exe").toURI().toString()))
        assertFalse(PhotoPreviewSources.accepts("data:text/html;base64,PGI+"))
        assertFalse(PhotoPreviewSources.accepts("data:image/png,raw"))
        assertFalse(PhotoPreviewSources.accepts("/tmp/a.png"))
        assertFalse(PhotoPreviewSources.accepts(""))
        assertFalse(PhotoPreviewSources.accepts(null))
        // 値が文字でない・画像でない値は集めない
        assertTrue(PhotoPreviewSources.photos(mapOf("p" to JSONValue.Num(1.0)), listOf(photo("p"))).isEmpty())
        assertTrue(PhotoPreviewSources.photos(mapOf("p" to JSONValue.Str("https://x/a.png")), listOf(photo("p"))).isEmpty())
    }

    // MARK: - 開く / 閉じる

    @Test
    fun openStartsAtTheClickedPhotoAndStepsWithinTheList() {
        val p = PhotoPreview()
        val list = listOf(PreviewPhoto(a, "前"), PreviewPhoto(b, "後"))
        assertFalse(p.isOpen)
        assertFalse("見せる物が無ければ開かない", p.open(emptyList()))
        assertTrue(p.open(list, start = b))
        assertEquals(1, p.index)
        assertFalse("最後から先へは行かない", p.next())
        assertTrue(p.previous())
        assertEquals(PreviewPhoto(a, "前"), p.current)
        assertFalse(p.previous())
        assertEquals("前(1 / 2)", p.title())
        p.close()
        assertFalse(p.isOpen)
        assertNull(p.current)
        // 並びに無い写真は 1 枚だけ・見せられない値は開かない
        assertTrue(p.openAt(list, File("/tmp/x.png").toURI().toString(), "部品"))
        assertEquals(1, p.photos.size)
        assertEquals("部品", p.title())
        p.close()
        assertFalse(p.openAt(list, "https://example.com/a.png", "部品"))
        assertTrue(p.openAt(list, b, "後"))
        assertEquals(1, p.index)
    }

    @Test
    fun escAndSpaceCloseArrowsStep() {
        assertEquals(PreviewKey.CLOSE, PhotoPreview.keyOf(KeyPress(Key.Escape)))
        assertEquals(PreviewKey.CLOSE, PhotoPreview.keyOf(KeyPress(Key.Spacebar)))
        assertEquals(PreviewKey.PREVIOUS, PhotoPreview.keyOf(KeyPress(Key.DirectionLeft)))
        assertEquals(PreviewKey.NEXT, PhotoPreview.keyOf(KeyPress(Key.DirectionRight)))
        assertNull(PhotoPreview.keyOf(KeyPress(Key.Spacebar, ctrl = true)))
        assertNull(PhotoPreview.keyOf(KeyPress(Key.A)))
    }

    @Test
    fun a11yNameCarriesTheFieldName() {
        assertEquals("写真のプレビュー: 前", A11y.previewTitle("前"))
        assertEquals("写真のプレビュー: 画像", A11y.previewTitle(""))
    }

    // MARK: - キーボード(recipes の写真)

    private fun recipeForm(): Pair<DesktopSession, DesktopKeyboard> {
        val s = DesktopSession(Examples.load("recipes"), MockStore())
        val k = DesktopKeyboard(s.dsl, s, HostOs.WINDOWS)
        k.openScreen("recipe_form")
        assertEquals("recipe_form", s.top.screen)
        return s to k
    }

    @Test
    fun spaceOpensOnlyWhenNothingInsideHasFocus() {
        val (s, k) = recipeForm()
        assertEquals(Shortcut.PREVIEW, Keymap.match(KeyPress(Key.Spacebar), HostOs.LINUX))
        assertEquals("Space", Keymap.label(Shortcut.PREVIEW, HostOs.WINDOWS))
        // 写真が無ければ Space は流す(入力欄などへ)
        k.rootFocused = true
        assertFalse(k.handle(KeyPress(Key.Spacebar), bubble = true))
        s.form(s.top)!!.values["photo"] = JSONValue.Str(a)
        // 入力欄・ボタンを選んでいる時は開かない
        k.rootFocused = false
        assertFalse(k.handle(KeyPress(Key.Spacebar), bubble = true))
        assertFalse(k.photos.isOpen)
        // 先の段(入力欄より前)では取らない
        k.rootFocused = true
        assertFalse(k.handle(KeyPress(Key.Spacebar), bubble = false))
        assertTrue(k.handle(KeyPress(Key.Spacebar), bubble = true))
        assertTrue(k.photos.isOpen)
        assertEquals(a, k.photos.current?.src)
        // 開いている間は他のショートカットを取らない(Esc はダイアログが受ける)
        assertFalse(k.handle(KeyPress(Key.Escape), bubble = true))
        assertEquals("recipe_form", s.top.screen)
        k.closePreview()
        assertFalse(k.photos.isOpen)
    }

    @Test
    fun paletteAndClickOpenTheRecordsPhoto() {
        val (s, k) = recipeForm()
        assertTrue(k.run(Shortcut.PREVIEW))
        assertEquals(KeyboardText.NO_PREVIEW, s.message)
        assertFalse(k.photos.isOpen)
        s.form(s.top)!!.values["photo"] = JSONValue.Str(a)
        assertTrue(k.commands().any { it.id == "shortcut:PREVIEW" && it.hint == "Space" })
        assertTrue(k.openPhoto(a, "写真"))
        assertEquals(listOf(PreviewPhoto(a, "写真")), k.photos.photos)
    }

    // MARK: - 既定のアプリで開く

    @Test
    fun exportWritesAPngIntoTheDragTempFolderOnly() {
        val f = PhotoOpen.export(PreviewPhoto(a, "料理/写真"))
        assertNotNull(f)
        f!!
        assertTrue("引き出しと同じ一時フォルダ", DragOut.isTemp(f))
        assertEquals("料理_写真.png", f.name)
        assertTrue(f.readBytes().contentEquals(png1x1))
        // 画像として読めない物は書かない
        assertNull(PhotoOpen.export(PreviewPhoto(b, "壊れた")))
        assertNull(PhotoOpen.export(PreviewPhoto("https://example.com/a.png", "外")))
        f.parentFile.deleteRecursively()
    }
}
