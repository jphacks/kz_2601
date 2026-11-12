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
import java.nio.file.Files
import java.nio.file.attribute.PosixFilePermissions
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
        // 開いている間は他のショートカットを取らない
        assertFalse(k.handle(KeyPress(Key.N, ctrl = true), bubble = true))
        assertFalse(k.handle(KeyPress(Key.Escape), bubble = false))
        assertTrue(k.photos.isOpen)
        assertEquals("recipe_form", s.top.screen)
        k.closePreview()
        assertFalse(k.photos.isOpen)
    }

    @Test
    fun closeKeysReachingTheRootClosePreviewWithoutLeavingTheScreen() {
        val (s, k) = recipeForm()
        s.form(s.top)!!.values["photo"] = JSONValue.Str(a)
        // フォーカスがダイアログの外にあり、閉じるキーが一番外まで来た時もプレビューを閉じる(画面は戻らない)
        for (key in listOf(Key.Escape, Key.Spacebar)) {
            assertTrue(k.openPreview())
            assertTrue(k.handle(KeyPress(key), bubble = true))
            assertFalse(k.photos.isOpen)
            assertEquals("recipe_form", s.top.screen)
        }
        // ← → は流す
        assertTrue(k.openPreview())
        assertFalse(k.handle(KeyPress(Key.DirectionRight), bubble = true))
        assertTrue(k.photos.isOpen)
    }

    @Test
    fun heldSpaceDoesNotOpenAndThenClose() {
        val (s, k) = recipeForm()
        s.form(s.top)!!.values["photo"] = JSONValue.Str(a)
        k.rootFocused = true
        // 押しっぱなしの繰り返しでは開かない
        assertFalse(k.handle(KeyPress(Key.Spacebar), bubble = true, repeat = true))
        assertFalse(k.photos.isOpen)
        assertTrue(k.handle(KeyPress(Key.Spacebar), bubble = true))
        assertTrue(k.photos.isOpen)
        // 開いた後の繰り返しでは閉じない(キーは受けて流さない)
        assertTrue(k.handle(KeyPress(Key.Spacebar), bubble = true, repeat = true))
        assertTrue(k.photos.isOpen)
    }

    @Test
    fun repeatFilterActsOnTheFirstKeyDownOnly() {
        val f = KeyRepeatFilter()
        val down1 = Any()
        assertFalse(f.isRepeat(down1, Key.Spacebar, down = true))
        // 同じ出来事を外側と内側で見ても同じ答え
        assertFalse(f.isRepeat(down1, Key.Spacebar, down = true))
        assertTrue(f.isRepeat(Any(), Key.Spacebar, down = true))
        assertTrue(f.isRepeat(Any(), Key.Spacebar, down = true))
        assertFalse(f.isRepeat(Any(), Key.Spacebar, down = false))
        assertFalse("離した後は 1 回目", f.isRepeat(Any(), Key.Spacebar, down = true))
    }

    @Test
    fun networkFileUrisAreNotShown() {
        assertTrue(PhotoPreviewSources.accepts("file:///tmp/a.png"))
        assertTrue(PhotoPreviewSources.accepts("file:/tmp/a.png"))
        assertTrue(PhotoPreviewSources.accepts("file:///C:/photos/a.png"))
        // UNC・ホスト付きの file URI は手元のファイルとして扱わない
        assertFalse(PhotoPreviewSources.accepts("file://host/share/a.png"))
        assertFalse(PhotoPreviewSources.accepts("file:////host/share/a.png"))
        assertFalse(PhotoPreviewSources.accepts("file://///host/share/a.png"))
        assertFalse(PhotoPreviewSources.accepts("file://localhost/tmp/a.png"))
        assertFalse(PhotoPreviewSources.accepts("file:/%5C%5Chost%5Cshare%5Ca.png"))
        assertFalse(PhotoPreviewSources.accepts("\\\\host\\share\\a.png"))
        assertFalse(PhotoPreviewSources.accepts("file:\\\\host\\share\\a.png"))
        assertNull(PhotoPreviewSources.localFile("file:////host/share/a.png"))
        // 「既定のアプリで開く」も書き出さない
        val root = Files.createTempDirectory("preview-root").toFile()
        try {
            assertNull(PhotoOpen.export(PreviewPhoto("file:////host/share/a.png", "n"), root))
            assertNull(PhotoOpen.export(PreviewPhoto("file://host/share/a.png", "n"), root))
            assertEquals(0, root.listFiles()!!.size)
        } finally {
            root.deleteRecursively()
        }
        // 並びに入らない・押しても開かない
        assertTrue(PhotoPreviewSources.photos(mapOf("p" to JSONValue.Str("file:////host/share/a.png")), listOf(photo("p"))).isEmpty())
        assertFalse(PhotoPreview().openAt(emptyList(), "file://host/share/a.png", "n"))
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
    fun exportWritesAPngIntoThePerUserFolderOnly() {
        val root = File(Files.createTempDirectory("preview-root").toFile(), "open")
        try {
            val f = PhotoOpen.export(PreviewPhoto(a, "料理/写真"), root)
            assertNotNull(f)
            f!!
            assertEquals("渡した場所の下", root.canonicalFile, f.parentFile.parentFile.canonicalFile)
            assertFalse("皆で使う引き出しの一時フォルダには置かない", DragOut.isTemp(f))
            assertEquals("料理_写真.png", f.name)
            assertTrue(f.readBytes().contentEquals(png1x1))
            if (root.toPath().fileSystem.supportedFileAttributeViews().contains("posix")) {
                assertEquals("rwx------", PosixFilePermissions.toString(Files.getPosixFilePermissions(root.toPath())))
            }
            // 画像として読めない物は書かない
            assertNull(PhotoOpen.export(PreviewPhoto(b, "壊れた"), root))
            assertNull(PhotoOpen.export(PreviewPhoto("https://example.com/a.png", "外"), root))
            // この端末のデータを消す時に消える
            PhotoOpen.clearTemp(root)
            assertFalse(f.exists())
            assertEquals(0, root.listFiles()!!.size)
        } finally {
            root.parentFile.deleteRecursively()
        }
    }

    @Test
    fun tempRootIsPerUser() {
        val data = File("/home/u/.local/share/tailorit")
        // Linux は XDG_RUNTIME_DIR(絶対パスの時だけ)
        assertEquals(File(File("/run/user/1000", "tailorit"), "open"), PhotoOpen.tempRoot("Linux", mapOf("XDG_RUNTIME_DIR" to "/run/user/1000")) { data })
        assertEquals(File(File(data, "tmp"), "open"), PhotoOpen.tempRoot("Linux", mapOf("XDG_RUNTIME_DIR" to "rel")) { data })
        assertEquals(File(File(data, "tmp"), "open"), PhotoOpen.tempRoot("Linux", emptyMap()) { data })
        // 他の OS は端末の保存先の下(XDG_RUNTIME_DIR は見ない)
        val win = File("C:\\Users\\u\\AppData\\Roaming\\TailorIt")
        assertEquals(File(File(win, "tmp"), "open"), PhotoOpen.tempRoot("Windows 11", mapOf("XDG_RUNTIME_DIR" to "/run/user/1000")) { win })
        val tmp = File(System.getProperty("java.io.tmpdir")).canonicalFile
        assertFalse(PhotoOpen.tempRoot("Linux", emptyMap()) { data }.startsWith(tmp))
    }

    @Test
    fun previewSizeImagesStayOutOfTheSharedCache() {
        ImageLoader.clear()
        try {
            assertNotNull(ImageLoader.load(a, allowFile = true, maxPx = 2048, withPng = false, useCache = false))
            assertEquals(0, ImageLoader.size())
            assertNull(ImageLoader.cached(a, allowFile = true, maxPx = 2048, withPng = false))
            // 一覧の読み方は今までどおり覚える
            assertNotNull(ImageLoader.load(a, allowFile = true, maxPx = ImageLoader.THUMBNAIL_PX))
            assertEquals(1, ImageLoader.size())
        } finally {
            ImageLoader.clear()
        }
    }
}
