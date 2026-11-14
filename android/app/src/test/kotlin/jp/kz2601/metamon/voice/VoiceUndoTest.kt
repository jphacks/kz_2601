package jp.kz2601.metamon.voice

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.AppDocument
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.services.AIClient
import jp.kz2601.metamon.services.ChatMessage
import jp.kz2601.metamon.services.ChatMode
import jp.kz2601.metamon.services.ChatTurn
import jp.kz2601.metamon.services.GenerateMode
import jp.kz2601.metamon.services.GenerateProgress
import jp.kz2601.metamon.services.MorphResult
import jp.kz2601.metamon.services.mock.MockSpeechTranscriber
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.IOException

/** 元に戻す / やり直す(#895・iOS の `AppRuntime.undo` / `redo`)。 */
class VoiceUndoTest {
    private val initial = AppDocument("app-1", 0, appDsl("Tailor It", voiceOnly = true))

    private fun appDsl(name: String, voiceOnly: Boolean = false): AppDSL = AppDSL.decode(
        if (voiceOnly) {
            """{"schemaVersion":1,"app":{"name":"$name","voiceEntry":"fullscreen"},"initialScreen":"voice",
               "screens":[{"id":"voice","components":[{"type":"voiceInput"}]}]}"""
        } else {
            """{"schemaVersion":1,"app":{"name":"$name"},"initialScreen":"home",
               "screens":[{"id":"home","title":"$name","components":[]}]}"""
        },
    )

    /** 生成のたびに版を 1 つ進め、アプリ名 = 依頼文の DSL を返す。 */
    private inner class FakeAI : AIClient {
        var version = 0

        override suspend fun morph(transcript: String, appId: String, currentVersion: Int, locale: String?, mode: GenerateMode): MorphResult =
            error("not used")

        override suspend fun morphStreaming(
            transcript: String,
            appId: String,
            currentVersion: Int,
            locale: String?,
            mode: GenerateMode,
            progress: (GenerateProgress) -> Unit,
        ): MorphResult = MorphResult(AppDocument(appId, ++version, appDsl(transcript)), summary = "${transcript}にしました", envelopeJson = "{}")

        override suspend fun chat(appId: String, messages: List<ChatMessage>, locale: String?, mode: ChatMode?): ChatTurn = error("not used")

        override suspend fun resetChat(appId: String) = Unit
    }

    /** サーバーの版の履歴。appconfig と同じく restore も新しい版(kind=restore・restoredFrom)を積む。 */
    private class FakeVersions(private val ai: FakeAI) : AppVersions {
        val names = mutableMapOf<Int, String>()
        val infos = mutableMapOf<Int, VersionInfo>()
        val restored = mutableListOf<Int>()
        var fail = false

        /** 生成・修正で増えた版を履歴に積む */
        fun record(version: Int, name: String) {
            names[version] = name
            infos[version] = VersionInfo(
                version = version,
                parentVersion = if (version > 1) version - 1 else null,
                kind = if (version == 1) "create" else "edit",
            )
        }

        override suspend fun versions(appId: String): List<VersionInfo> {
            if (fail) throw IOException("offline")
            return infos.keys.sortedDescending().map { infos.getValue(it) }
        }

        override suspend fun restore(appId: String, version: Int): AppDocument {
            if (fail) throw IOException("offline")
            restored += version
            val parent = ai.version
            val v = ++ai.version
            names[v] = names.getValue(version)
            infos[v] = VersionInfo(v, parentVersion = parent, kind = "restore", restoredFrom = version)
            return AppDocument(appId, v, AppDSL.decode(
                """{"schemaVersion":1,"app":{"name":"${names.getValue(version)}"},"initialScreen":"home",
                   "screens":[{"id":"home","components":[]}]}""",
            ))
        }
    }

    private fun TestScope.vm(ai: FakeAI, versions: AppVersions? = null) = VoiceViewModel(
        transcriber = MockSpeechTranscriber(),
        ai = ai,
        appId = { "app-1" },
        initial = initial,
        clarifyEnabled = { false },
        versions = versions,
        scope = CoroutineScope(StandardTestDispatcher(testScheduler) + Job()),
    )

    private val VoiceViewModel.appName get() = state.value.document.dsl.app.name

    private fun TestScope.generate(vm: VoiceViewModel, text: String, versions: FakeVersions? = null) {
        vm.generateNow(text)
        advanceUntilIdle()
        versions?.record(vm.state.value.document.version, text)
    }

    @Test
    fun firstAppHasNothingToUndo() = runTest {
        val vm = vm(FakeAI())
        generate(vm, "家計簿")
        assertFalse(vm.state.value.canUndo)
        assertNull("最初のアプリは戻す先が無いのでボタンを出さない", vm.state.value.summaryAction)

        vm.undo()
        advanceUntilIdle()
        assertEquals(UndoNotice.NO_EARLIER, vm.state.value.notice)
        assertEquals("家計簿", vm.appName)
        vm.dismissNotice()
        assertNull(vm.state.value.notice)
    }

    @Test
    fun localUndoThenRedo() = runTest {
        val vm = vm(FakeAI())
        generate(vm, "家計簿")
        generate(vm, "家計簿・グラフ付き")
        val edited = vm.state.value
        assertTrue(edited.canUndo)
        assertEquals(UndoAction.UNDO, edited.summaryAction)
        assertEquals("家計簿・グラフ付きにしました", edited.summary)

        vm.undo(expectedVersion = edited.document.version)
        advanceUntilIdle()
        val undone = vm.state.value
        assertEquals("家計簿", vm.appName)
        assertEquals(1, undone.document.version)
        assertEquals(UndoAction.UNDO, undone.undone)
        assertEquals("戻した直後は「やり直す」", UndoAction.REDO, undone.summaryAction)
        assertNull(undone.summary)
        assertTrue(undone.canRedo)
        assertFalse(undone.canUndo)

        vm.redo(expectedVersion = undone.document.version)
        advanceUntilIdle()
        val redone = vm.state.value
        assertEquals("家計簿・グラフ付き", vm.appName)
        assertEquals(UndoAction.REDO, redone.undone)
        assertFalse(redone.canRedo)
        assertTrue("やり直した後はもう一度戻せる", redone.canUndo)
        assertEquals(UndoAction.UNDO, redone.summaryAction)
    }

    @Test
    fun staleVersionIsIgnored() = runTest {
        val vm = vm(FakeAI())
        generate(vm, "家計簿")
        generate(vm, "家計簿・グラフ付き")
        val pressedAt = vm.state.value.document.version
        vm.undo(pressedAt)
        advanceUntilIdle()
        // 連打の 2 回目(押した時点の版はもう古い)
        vm.undo(pressedAt)
        advanceUntilIdle()
        assertEquals("家計簿", vm.appName)
        assertNull(vm.state.value.notice)
    }

    @Test
    fun newChangeAfterUndoDropsRedo() = runTest {
        val vm = vm(FakeAI())
        generate(vm, "家計簿")
        generate(vm, "家計簿・グラフ付き")
        vm.undo()
        advanceUntilIdle()
        assertTrue(vm.state.value.canRedo)

        generate(vm, "家計簿・色を青に")
        assertFalse(vm.state.value.canRedo)
        vm.redo()
        advanceUntilIdle()
        assertEquals("家計簿・色を青に", vm.appName)
    }

    @Test
    fun serverUndoRestoresPreviousVersionAndRedoRestoresTheOneBefore() = runTest {
        val ai = FakeAI()
        val versions = FakeVersions(ai)
        val vm = vm(ai, versions)
        generate(vm, "家計簿", versions) // v1
        assertFalse(vm.state.value.canUndo)
        generate(vm, "家計簿・グラフ付き", versions) // v2
        assertTrue(vm.state.value.canUndo)

        vm.undo()
        advanceUntilIdle()
        assertEquals(listOf(1), versions.restored)
        assertEquals("restore は新しい版を作る", 3, vm.state.value.document.version)
        assertEquals("家計簿", vm.appName)
        assertTrue(vm.state.value.canRedo)

        vm.redo()
        advanceUntilIdle()
        assertEquals(listOf(1, 2), versions.restored)
        assertEquals(4, vm.state.value.document.version)
        assertEquals("家計簿・グラフ付き", vm.appName)
        assertFalse(vm.state.value.canRedo)
    }

    @Test
    fun serverFailureFallsBackToLocalHistory() = runTest {
        val ai = FakeAI()
        val versions = FakeVersions(ai)
        val vm = vm(ai, versions)
        generate(vm, "家計簿", versions)
        generate(vm, "家計簿・グラフ付き", versions)
        versions.fail = true

        vm.undo()
        advanceUntilIdle()
        assertEquals("家計簿", vm.appName)
        assertTrue(versions.restored.isEmpty())

        // 端末の履歴で戻したので、やり直しはサーバーに送らない
        vm.redo()
        advanceUntilIdle()
        assertEquals("家計簿・グラフ付き", vm.appName)
        assertNull(vm.state.value.notice)
    }

    @Test
    fun serverRedoFailureShowsNotice() = runTest {
        val ai = FakeAI()
        val versions = FakeVersions(ai)
        val vm = vm(ai, versions)
        generate(vm, "家計簿", versions)
        generate(vm, "家計簿・グラフ付き", versions)
        vm.undo()
        advanceUntilIdle()
        versions.fail = true

        vm.redo()
        advanceUntilIdle()
        assertEquals(UndoNotice.REDO_FAILED, vm.state.value.notice)
        assertEquals("家計簿", vm.appName)
        assertFalse(vm.state.value.undoing)
        assertTrue("失敗したらもう一度押せる", vm.state.value.canRedo)
    }

    @Test
    fun dismissSummaryClearsUndoButton() = runTest {
        val vm = vm(FakeAI())
        generate(vm, "家計簿")
        generate(vm, "家計簿・グラフ付き")
        vm.dismissSummary()
        assertNull(vm.state.value.summary)
        assertNull(vm.state.value.summaryAction)
        assertTrue("シートの「元に戻す」はまだ押せる", vm.state.value.canUndo)
    }

    @Test
    fun switchingAppsDropsThePreviousAppsUndoHistory() = runTest {
        val vm = vm(FakeAI())
        generate(vm, "家計簿")
        generate(vm, "家計簿・グラフ付き")
        assertTrue(vm.state.value.canUndo)

        // 一覧から別のアプリへ切り替えた(ランタイムが開いたアプリを映す)
        vm.adopt(AppDocument("app-2", 3, appDsl("割り勘")))
        val switched = vm.state.value
        assertFalse("前のアプリの履歴で「元に戻す」を出さない", switched.canUndo)
        assertFalse(switched.canRedo)
        assertNull(switched.summaryAction)

        vm.undo()
        advanceUntilIdle()
        assertEquals("前のアプリの版を当てない", "app-2", vm.state.value.document.appId)
        assertEquals("割り勘", vm.appName)
        assertEquals(UndoNotice.NO_EARLIER, vm.state.value.notice)
    }

    // MARK: 続けて戻す / やり直す(#1786・iOS の UndoWalk と同じ選び方)

    /** 完了の条件 1: 3 回直した後に 3 回戻すと最初の版の中身になる(直前 2 版の往復にならない)。 */
    @Test
    fun threeUndosWalkBackToTheFirstVersion() = runTest {
        val ai = FakeAI()
        val versions = FakeVersions(ai)
        val vm = vm(ai, versions)
        generate(vm, "A", versions) // v1
        generate(vm, "B", versions) // v2
        generate(vm, "C", versions) // v3
        generate(vm, "D", versions) // v4
        assertEquals(4, vm.state.value.document.version)

        vm.undo(vm.state.value.document.version)
        advanceUntilIdle()
        assertEquals("C", vm.appName)
        vm.undo(vm.state.value.document.version)
        advanceUntilIdle()
        assertEquals("直前の D に戻らず、さらに前へ", "B", vm.appName)
        vm.undo(vm.state.value.document.version)
        advanceUntilIdle()
        assertEquals("A", vm.appName)
        assertEquals("restore する版は減っていく", listOf(3, 2, 1), versions.restored)
        assertEquals("restore は新しい版を作る", 7, vm.state.value.document.version)

        vm.undo(vm.state.value.document.version)
        advanceUntilIdle()
        assertEquals(UndoNotice.NO_EARLIER, vm.state.value.notice)
        assertEquals("最初の版より前へは restore しない", 3, versions.restored.size)
        assertEquals("A", vm.appName)
    }

    /** 完了の条件 2: 2 回戻した後に 2 回やり直せる。 */
    @Test
    fun twoUndosThenTwoRedos() = runTest {
        val ai = FakeAI()
        val versions = FakeVersions(ai)
        val vm = vm(ai, versions)
        generate(vm, "A", versions)
        generate(vm, "B", versions)
        generate(vm, "C", versions)
        generate(vm, "D", versions)

        vm.undo(vm.state.value.document.version)
        advanceUntilIdle()
        vm.undo(vm.state.value.document.version)
        advanceUntilIdle()
        assertEquals("B", vm.appName)
        assertTrue(vm.state.value.canRedo)

        vm.redo(vm.state.value.document.version)
        advanceUntilIdle()
        assertEquals("C", vm.appName)
        assertTrue("もう 1 つやり直せる", vm.state.value.canRedo)

        vm.redo(vm.state.value.document.version)
        advanceUntilIdle()
        assertEquals("D", vm.appName)
        assertFalse(vm.state.value.canRedo)
        assertTrue("やり直した後はもう一度戻せる", vm.state.value.canUndo)

        // 戻す → やり直す → 戻す も往復しない
        vm.undo(vm.state.value.document.version)
        advanceUntilIdle()
        assertEquals("C", vm.appName)
        vm.undo(vm.state.value.document.version)
        advanceUntilIdle()
        assertEquals("B", vm.appName)
    }

    /** 完了の条件 3: 同じ版を見て押された連打は restore 1 回。 */
    @Test
    fun rapidTapsRestoreOnce() = runTest {
        val ai = FakeAI()
        val versions = FakeVersions(ai)
        val vm = vm(ai, versions)
        generate(vm, "A", versions)
        generate(vm, "B", versions)
        generate(vm, "C", versions)

        val pressedAt = vm.state.value.document.version
        vm.undo(pressedAt)
        vm.undo(pressedAt)
        vm.undo(pressedAt)
        advanceUntilIdle()
        assertEquals("同じ版を見て押した連打は 1 回", listOf(2), versions.restored)
        assertEquals("B", vm.appName)

        // 画面が変わってから押し直せば、さらに前へ
        vm.undo(vm.state.value.document.version)
        advanceUntilIdle()
        assertEquals("A", vm.appName)
        assertEquals(listOf(2, 1), versions.restored)
    }

    /** 戻した後に新しく直したら、その修正の前(戻した先の中身)へ戻り、やり直すは消える。 */
    @Test
    fun editAfterUndoClearsRedoAndUndoesToTheRestoredContent() = runTest {
        val ai = FakeAI()
        val versions = FakeVersions(ai)
        val vm = vm(ai, versions)
        generate(vm, "A", versions)
        generate(vm, "B", versions)
        generate(vm, "C", versions)
        vm.undo(vm.state.value.document.version)
        advanceUntilIdle()
        vm.undo(vm.state.value.document.version)
        advanceUntilIdle()
        assertEquals("A", vm.appName)
        assertTrue(vm.state.value.canRedo)

        generate(vm, "A2", versions)
        assertFalse("新しい修正で「やり直す」は消える", vm.state.value.canRedo)

        vm.undo(vm.state.value.document.version)
        advanceUntilIdle()
        assertEquals("修正の前の中身へ", "A", vm.appName)
        assertEquals(1, versions.restored.last())
    }
}
