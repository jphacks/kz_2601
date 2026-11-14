package jp.kz2601.metamon.voice

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.AppDocument
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.services.AIClient
import jp.kz2601.metamon.services.ChatMessage
import jp.kz2601.metamon.services.ChatMode
import jp.kz2601.metamon.services.ChatOption
import jp.kz2601.metamon.services.ChatPaidFeature
import jp.kz2601.metamon.services.ChatTurn
import jp.kz2601.metamon.services.GenerateMode
import jp.kz2601.metamon.services.GenerateProgress
import jp.kz2601.metamon.services.MorphResult
import jp.kz2601.metamon.services.mock.MockSpeechTranscriber
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.Job
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder

/** 生成の前の相談(複数選択・質問 n / 3・相談の続き)と、アプリごとの会話の履歴(#893)。 */
@OptIn(ExperimentalCoroutinesApi::class)
class VoiceConsultTest {
    @get:Rule
    val tmp = TemporaryFolder()

    private fun doc(appId: String, version: Int, voiceOnly: Boolean) = AppDocument(
        appId = appId,
        version = version,
        dsl = AppDSL.decode(
            if (voiceOnly) {
                """{"schemaVersion":1,"app":{"name":"Tailor It","voiceEntry":"fullscreen"},"initialScreen":"voice",
                   "screens":[{"id":"voice","components":[{"type":"voiceInput"}]}]}"""
            } else {
                """{"schemaVersion":1,"app":{"name":"家計簿"},"initialScreen":"home",
                   "screens":[{"id":"home","title":"家計簿","components":[]}]}"""
            },
        ),
    )

    /** 呼ばれた内容を残す偽物。chat の返事は [turns] から順に、generate は版を 1 つ進める。 */
    private class FakeAI : AIClient {
        val chatCalls = mutableListOf<List<ChatMessage>>()
        val chatApps = mutableListOf<String>()
        val morphCalls = mutableListOf<String>()
        val resets = mutableListOf<String>()
        val turns = ArrayDeque<ChatTurn>()
        var current: AppDocument? = null

        override suspend fun morph(transcript: String, appId: String, currentVersion: Int, locale: String?, mode: GenerateMode): MorphResult =
            error("not used")

        override suspend fun morphStreaming(
            transcript: String,
            appId: String,
            currentVersion: Int,
            locale: String?,
            mode: GenerateMode,
            progress: (GenerateProgress) -> Unit,
        ): MorphResult {
            morphCalls += transcript
            val next = current!!.copy(appId = appId, version = currentVersion + 1)
            return MorphResult(document = next, summary = "「$transcript」を反映", envelopeJson = "{}")
        }

        override suspend fun chat(appId: String, messages: List<ChatMessage>, locale: String?, mode: ChatMode?): ChatTurn {
            chatCalls += messages
            chatApps += appId
            return turns.removeFirst()
        }

        override suspend fun resetChat(appId: String) {
            resets += appId
        }
    }

    private class Env(val vm: VoiceViewModel, val store: ChatHistoryStore, var appId: String)

    private fun TestScope.env(ai: FakeAI, initial: AppDocument, store: ChatHistoryStore = ChatHistoryStore(tmp.root)): Env {
        val dispatcher = StandardTestDispatcher(testScheduler)
        lateinit var env: Env
        val vm = VoiceViewModel(
            transcriber = MockSpeechTranscriber(),
            ai = ai,
            appId = { env.appId },
            initial = initial,
            clarifyEnabled = { true },
            chatHistory = store,
            historyHeader = "HEADER",
            io = dispatcher,
            scope = CoroutineScope(dispatcher + Job()),
        )
        env = Env(vm, store, initial.appId)
        return env
    }

    private fun question(text: String, turn: Int, vararg chips: String, multi: Boolean = false) =
        ChatTurn(kind = ChatTurn.Kind.QUESTION, text = text, suggestions = chips.toList(), turn = turn, multiSelect = if (multi) true else null)

    // MARK: - 複数選択

    @Test
    fun joinedInstructionAndAnswer() {
        assertEquals("大きく", MultiSelect.joinedInstruction(listOf(" 大きく ", "")))
        assertEquals("・大きく\n・グラフ", MultiSelect.joinedInstruction(listOf("大きく", "グラフ")))
        assertEquals("支出、収入", MultiSelect.joinedAnswer(listOf("支出", " ", "収入")))
    }

    @Test
    fun combinedMergesOptionsAndPaidFeatures() {
        val a = ChatOption(id = "a", title = "大きく", instruction = "大きくして")
        val b = ChatOption(
            id = "b", title = "共有", instruction = "みんなで共有", requiresPro = true,
            paidFeatures = listOf(ChatPaidFeature("sharedData", "共有")), freeInstruction = "自分だけで記録",
        )
        val c = ChatOption(id = "c", title = "共有 2", instruction = "共有 2", requiresPro = true, paidFeatures = listOf(ChatPaidFeature("sharedData", "共有")))
        assertEquals(a, MultiSelect.combined(listOf(a)))
        assertNull(MultiSelect.combined(emptyList()))
        val all = MultiSelect.combined(listOf(a, b, c))!!
        assertEquals("a+b+c", all.optionId)
        assertEquals("大きく、共有、共有 2", all.title)
        assertEquals("・大きくして\n・みんなで共有\n・共有 2", all.instructionText)
        assertEquals(true, all.requiresPro)
        assertEquals(listOf("sharedData"), all.paidFeatures?.map { it.id })
        // 有料の案は無料版、無料の案は元の指示(無料版が無い有料の案は落とす)
        assertEquals("・大きくして\n・自分だけで記録", all.freeInstruction)
        val free = MultiSelect.combined(listOf(a, ChatOption(id = "d", title = "色", instruction = "青く")))!!
        assertNull(free.requiresPro)
        assertNull(free.freeInstruction)
    }

    @Test
    fun chooseManyBuildsWithCombinedInstruction() = runTest {
        val ai = FakeAI()
        val e = env(ai, doc("app-1", 0, voiceOnly = true))
        ai.current = doc("app-1", 0, voiceOnly = false)
        val a = ChatOption(id = "a", title = "支出", instruction = "支出を記録")
        val b = ChatOption(id = "b", title = "グラフ", instruction = "月のグラフ")
        ai.turns += ChatTurn(kind = ChatTurn.Kind.PROPOSAL, text = "案です", options = listOf(a, b), multiSelect = true, turn = 1)
        e.vm.submit("家計簿")
        runCurrent()
        assertTrue(e.vm.state.value.chatTurn!!.isMultiSelect)
        e.vm.chooseMany(emptyList()) // 0 件は押せない
        e.vm.chooseMany(listOf(a, b))
        advanceUntilIdle()
        assertEquals(listOf("・支出を記録\n・月のグラフ"), ai.morphCalls)
    }

    @Test
    fun answerManySendsChipsAsOneAnswer() = runTest {
        val ai = FakeAI()
        val e = env(ai, doc("app-1", 0, voiceOnly = true))
        ai.turns += question("何を記録?", 1, "支出", "収入", "貯金", multi = true)
        ai.turns += question("誰と使う?", 2)
        e.vm.submit("家計簿")
        runCurrent()
        e.vm.answerMany(listOf(" ", ""))
        runCurrent()
        assertEquals(1, ai.chatCalls.size) // 何も選んでいなければ送らない
        e.vm.answerMany(listOf("収入", "支出"))
        runCurrent()
        assertEquals("収入、支出", ai.chatCalls.last().last().text)
    }

    // MARK: - 質問 n / 3

    @Test
    fun questionProgressCountsQuestionsInCreateOnly() {
        val chat = listOf(
            ChatBubble(true, "家計簿"),
            ChatBubble(false, "何を?", ChatTurn.Kind.QUESTION),
            ChatBubble(true, "支出"),
            ChatBubble(false, "誰と?", ChatTurn.Kind.QUESTION),
        )
        assertEquals(2, QuestionProgress.number(chat, ChatMode.CREATE, ChatTurn.Kind.QUESTION))
        assertNull(QuestionProgress.number(chat, ChatMode.EDIT, ChatTurn.Kind.QUESTION))
        assertNull(QuestionProgress.number(chat + ChatBubble(true, "自分"), ChatMode.CREATE, ChatTurn.Kind.QUESTION))
        assertNull(QuestionProgress.number(chat.take(3) + ChatBubble(false, "OK", ChatTurn.Kind.READY), ChatMode.CREATE, ChatTurn.Kind.READY))
        val many = (1..4).flatMap { listOf(ChatBubble(true, "a$it"), ChatBubble(false, "q$it", ChatTurn.Kind.QUESTION)) }
        assertNull("上限を超えたら出さない", QuestionProgress.number(many, ChatMode.CREATE, ChatTurn.Kind.QUESTION))
    }

    @Test
    fun shortcutDependsOnMode() {
        val q = question("?", 1)
        val ready = ChatTurn(kind = ChatTurn.Kind.READY, text = "OK")
        val answer = ChatTurn(kind = ChatTurn.Kind.ANSWER, text = "はい")
        assertEquals(ChatShortcut.BUILD_NOW, ChatShortcut.of(q, ChatMode.CREATE))
        assertEquals(ChatShortcut.BUILD_NOW, ChatShortcut.of(answer, ChatMode.CREATE))
        assertEquals(ChatShortcut.NONE, ChatShortcut.of(ready, ChatMode.CREATE))
        assertEquals(ChatShortcut.APPLY_NOW, ChatShortcut.of(q, ChatMode.EDIT))
        assertEquals(ChatShortcut.NONE, ChatShortcut.of(answer, ChatMode.EDIT))
    }

    // MARK: - サーバーの会話

    @Test
    fun staleServerConversationIsClearedAndResent() = runTest {
        val ai = FakeAI()
        val e = env(ai, doc("app-1", 0, voiceOnly = true))
        ai.turns += question("古い会話に混ざった", 3)
        ai.turns += question("何を?", 1)
        e.vm.submit("家計簿")
        runCurrent()
        assertEquals(listOf("app-1"), ai.resets)
        assertEquals(2, ai.chatCalls.size)
        assertEquals("何を?", e.vm.state.value.chat.last().text)
    }

    // MARK: - 履歴

    @Test
    fun appliedConversationIsKeptWithDividerAndSurvivesReload() = runTest {
        val ai = FakeAI()
        val e = env(ai, doc("app-1", 0, voiceOnly = true))
        ai.current = doc("app-1", 0, voiceOnly = false)
        ai.turns += question("何を?", 1, "支出")
        ai.turns += ChatTurn(kind = ChatTurn.Kind.READY, text = "まとめました", transcript = "支出の家計簿", turn = 2)
        e.vm.submit("家計簿")
        runCurrent()
        e.vm.answer("支出")
        runCurrent()
        e.vm.buildFromChat()
        advanceUntilIdle()

        val history = e.vm.state.value.history
        assertEquals(
            listOf("家計簿", "何を?", "支出", "まとめました", "「支出の家計簿」を反映"),
            history.map { it.text },
        )
        assertEquals(ChatHistoryEntry.Kind.APPLIED, history.last().kind)
        assertEquals(1, history.last().version)
        assertTrue(e.vm.state.value.chat.isEmpty())
        assertTrue("閉じた会話があればシートは吹き出しの画面", e.vm.state.value.showsChat)

        // 端末から読み直しても同じ(別の保存先 = 起動し直し)
        val reloaded = ChatHistoryStore(tmp.root).load("app-1")
        assertEquals(history.map { it.kind to it.text }, reloaded.map { it.kind to it.text })
        assertEquals(1, reloaded.last().version)
    }

    @Test
    fun editContextSendsSummariesAndOnlyTheUnappliedTail() = runTest {
        val ai = FakeAI()
        val store = ChatHistoryStore(tmp.root)
        store.save(
            listOf(
                ChatHistoryEntry(ChatHistoryEntry.Kind.USER, "家計簿"),
                ChatHistoryEntry(ChatHistoryEntry.Kind.ASSISTANT, "まとめました"),
                ChatHistoryEntry(ChatHistoryEntry.Kind.APPLIED, "家計簿を作成", 1),
                ChatHistoryEntry(ChatHistoryEntry.Kind.USER, "色を変えたい"),
                ChatHistoryEntry(ChatHistoryEntry.Kind.ASSISTANT, "何色に?"),
            ),
            "app-1",
        )
        val e = env(ai, doc("app-1", 1, voiceOnly = false), store)
        advanceUntilIdle()
        assertEquals(5, e.vm.state.value.history.size)
        ai.turns += question("どこを?", 2)
        e.vm.submit("青にして")
        runCurrent()
        assertEquals(
            listOf("HEADER\n- v1: 家計簿を作成", "色を変えたい", "何色に?", "青にして"),
            ai.chatCalls.single().map { it.text },
        )
        assertEquals(ChatMessage.Role.ASSISTANT, ai.chatCalls.single().first().role)
    }

    @Test
    fun switchingAppsArchivesTheLiveChatAndSwapsHistory() = runTest {
        val ai = FakeAI()
        val store = ChatHistoryStore(tmp.root)
        store.save(listOf(ChatHistoryEntry(ChatHistoryEntry.Kind.APPLIED, "B を作成", 4)), "app-b")
        val e = env(ai, doc("app-a", 2, voiceOnly = false), store)
        ai.turns += question("どの画面?", 1)
        e.vm.submit("ボタンを大きく")
        runCurrent()
        assertEquals(2, e.vm.state.value.chat.size)

        e.appId = "app-b"
        e.vm.adopt(doc("app-b", 4, voiceOnly = false))
        advanceUntilIdle()
        assertTrue(e.vm.state.value.chat.isEmpty())
        assertEquals(listOf("B を作成"), e.vm.state.value.history.map { it.text })
        assertEquals(listOf("app-a"), ai.resets) // 前のアプリのサーバーの会話を消す
        // 前のアプリの会話は閉じた会話として残る
        assertEquals(listOf("ボタンを大きく", "どの画面?"), store.load("app-a").map { it.text })

        e.appId = "app-a"
        e.vm.adopt(doc("app-a", 2, voiceOnly = false))
        advanceUntilIdle()
        assertEquals(listOf("ボタンを大きく", "どの画面?"), e.vm.state.value.history.map { it.text })
    }

    @Test
    fun restartAsksBeforeClearingHistory() = runTest {
        val ai = FakeAI()
        val store = ChatHistoryStore(tmp.root)
        store.save(listOf(ChatHistoryEntry(ChatHistoryEntry.Kind.APPLIED, "作成", 1)), "app-1")
        val e = env(ai, doc("app-1", 1, voiceOnly = false), store)
        advanceUntilIdle()
        ai.turns += question("どこを?", 2)
        e.vm.submit("青に")
        runCurrent()

        e.vm.restartChat()
        assertTrue(e.vm.state.value.restartConfirm)
        e.vm.restartLiveChat()
        advanceUntilIdle()
        assertFalse(e.vm.state.value.restartConfirm)
        assertTrue(e.vm.state.value.chat.isEmpty())
        assertEquals(listOf("作成"), store.load("app-1").map { it.text }) // 今の会話だけ消えた

        e.vm.restartChat()
        e.vm.clearChatHistory()
        advanceUntilIdle()
        assertTrue(e.vm.state.value.history.isEmpty())
        assertTrue(ChatHistoryStore(tmp.root).load("app-1").isEmpty())
    }

    @Test
    fun restartWithoutHistoryClearsWithoutAsking() = runTest {
        val ai = FakeAI()
        val e = env(ai, doc("app-1", 0, voiceOnly = true))
        ai.turns += question("何を?", 1)
        e.vm.submit("家計簿")
        runCurrent()
        e.vm.restartChat()
        advanceUntilIdle()
        assertFalse(e.vm.state.value.restartConfirm)
        assertTrue(e.vm.state.value.chat.isEmpty())
        assertTrue(e.store.load("app-1").isEmpty())
    }

    @Test
    fun clarifyOffAppliesRightAwayWithoutHistory() = runTest {
        val ai = FakeAI()
        val dispatcher = StandardTestDispatcher(testScheduler)
        var clarify = true
        val vm = VoiceViewModel(
            transcriber = MockSpeechTranscriber(),
            ai = ai,
            appId = { "app-1" },
            initial = doc("app-1", 1, voiceOnly = false),
            clarifyEnabled = { clarify },
            io = dispatcher,
            scope = CoroutineScope(dispatcher + Job()),
        )
        ai.current = doc("app-1", 1, voiceOnly = false)
        clarify = false // 設定を切り替えたらすぐ効く
        vm.submit("青にして")
        advanceUntilIdle()
        assertEquals(listOf("青にして"), ai.morphCalls)
        assertTrue(ai.chatCalls.isEmpty())
        assertTrue("相談を使っていないアプリには区切りを入れない", vm.state.value.history.isEmpty())
    }

    // MARK: - 保存先

    @Test
    fun storeKeepsLatestEntriesAndSurvivesBrokenFiles() {
        val store = ChatHistoryStore(tmp.root)
        val many = (1..ChatHistoryStore.LIMIT + 5).map { ChatHistoryEntry(ChatHistoryEntry.Kind.USER, "m$it") }
        store.save(many, "app/1")
        val loaded = ChatHistoryStore(tmp.root).load("app/1")
        assertEquals(ChatHistoryStore.LIMIT, loaded.size)
        assertEquals("m6", loaded.first().text)

        tmp.root.resolve("broken.json").writeText("{not json")
        assertTrue(ChatHistoryStore(tmp.root).load("broken").isEmpty())
        assertFalse(tmp.root.resolve("broken.json").exists())

        store.save(emptyList(), "app/1")
        assertTrue(ChatHistoryStore(tmp.root).load("app/1").isEmpty())
        store.save(many.take(1), "x")
        store.removeAll()
        assertTrue(ChatHistoryStore(tmp.root).load("x").isEmpty())
    }

    @Test
    fun contextKeepsOnlyLastFiveSummaries() {
        val history = (1..7).map { ChatHistoryEntry(ChatHistoryEntry.Kind.APPLIED, if (it == 7) "" else "変更$it", it) }
        val messages = ChatContext.messages(history, listOf(ChatMessage(ChatMessage.Role.USER, "次")), "H")
        assertEquals("H\n- v3: 変更3\n- v4: 変更4\n- v5: 変更5\n- v6: 変更6\n- v7", messages.first().text)
        assertEquals("次", messages.last().text)
        assertEquals(2, messages.size)
        assertEquals(listOf("今"), ChatContext.messages(emptyList(), listOf(ChatMessage(ChatMessage.Role.USER, "今")), "H").map { it.text })
    }
}
