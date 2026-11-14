package jp.kz2601.metamon.desktop.share

import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.StringTable
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.services.InstallResult
import jp.kz2601.metamon.services.SharePreview
import jp.kz2601.metamon.services.http.ApiError
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.JsonObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.IOException

/** 記録を共有しないアプリのシェアリンク: 取り込む前に「〈名前〉を追加しますか?」と聞く(#3364)。 */
class AddShare3364Test {
    private class FakeImporter(
        var preview: () -> SharePreview = { SharePreview(token = "t1", name = "家計簿") },
        var import: () -> InstallResult = { InstallResult(id = "a9", name = "家計簿", dsl = JsonObject(emptyMap())) },
    ) : ShareImporter {
        val imported = mutableListOf<String>()
        override suspend fun preview(token: String) = preview()
        override suspend fun import(token: String): InstallResult {
            imported += token
            return import()
        }
    }

    private fun ask(flow: AddShareFlow, token: String = "t1") = runBlocking { flow.open(token) } as AddShareFlow.Opened.AskToAdd

    @Test
    fun withNameAsksByNameAndAddOpensThatApp() = runBlocking {
        val importer = FakeImporter()
        val flow = AddShareFlow(importer)
        val pending = ask(flow).pending
        assertEquals("「家計簿」を追加しますか?", Strings.withLang(Lang.JA) { pending.title() })
        assertTrue("聞く前に取り込まない", importer.imported.isEmpty())

        val added = flow.add(pending) as AddShareFlow.Added.Done
        assertEquals("a9", added.app.id)
        assertEquals(listOf("t1"), importer.imported)
    }

    @Test
    fun withoutPreviewAsksWithoutAName() {
        val flow = AddShareFlow(FakeImporter(preview = { throw IOException("offline") }))
        val pending = ask(flow).pending
        assertNull(pending.preview)
        assertEquals("シェアされたアプリを追加しますか?", Strings.withLang(Lang.JA) { pending.title() })
        assertEquals(Strings.withLang(Lang.EN) { Strings["share_add_title_unnamed"] }, Strings.withLang(Lang.EN) { pending.title() })
        val blank = ask(AddShareFlow(FakeImporter(preview = { SharePreview(token = "t1", name = " ") }))).pending
        assertEquals("名前が空でも名前なし", Strings.withLang(Lang.JA) { Strings["share_add_title_unnamed"] }, Strings.withLang(Lang.JA) { blank.title() })
    }

    @Test
    fun cancelDoesNotImport() {
        val importer = FakeImporter()
        ask(AddShareFlow(importer))
        // 「追加しない」は add を呼ばずにダイアログを閉じるだけ
        assertTrue(importer.imported.isEmpty())
    }

    @Test
    fun sharedDataAppGoesToTheExistingJoinFlow() = runBlocking {
        val importer = FakeImporter(preview = { SharePreview(token = "t1", name = "部活", sharedData = true) })
        val opened = AddShareFlow(importer).open("t1")
        assertTrue(opened is AddShareFlow.Opened.Join)
        // 自分のアプリなら参加ではなく追加を聞く
        val mine = AddShareFlow(FakeImporter(preview = { SharePreview(token = "t1", name = "部活", sharedData = true, mine = true) })).open("t1")
        assertTrue(mine is AddShareFlow.Opened.AskToAdd)
    }

    @Test
    fun addFailureKeepsTheDialogWithAReason() = runBlocking {
        val flow = AddShareFlow(FakeImporter(import = { throw ApiError.Http(404, "{}") }))
        val r = flow.add(ask(flow).pending) as AddShareFlow.Added.Failed
        assertEquals(AddShareFailure.REVOKED, r.reason)
        val offline = AddShareFlow(FakeImporter(import = { throw IOException("down") }))
        assertEquals(AddShareFailure.OFFLINE, (offline.add(ask(offline).pending) as AddShareFlow.Added.Failed).reason)
    }

    @Test
    fun mockConfigurationCannotOpenLinks() = runBlocking {
        assertEquals(AddShareFlow.Opened.Failed(AddShareFailure.UNAVAILABLE), AddShareFlow(null).open("t1"))
    }

    @Test
    fun stringsResolveInBothLanguages() {
        val t = StringTable.fromClasspath()
        val keys = AddShareFailure.entries.map { it.key } +
            listOf("share_add_cancel", "share_add_confirm", "share_add_note", "share_add_title", "share_add_title_unnamed")
        for (lang in Lang.entries) for (k in keys) assertTrue("$k ($lang)", t.hasString(lang, k))
    }
}
