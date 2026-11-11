package jp.kz2601.metamon.voice

import jp.kz2601.metamon.R
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
import jp.kz2601.metamon.services.MyUsage
import jp.kz2601.metamon.services.PlanUsageSource
import jp.kz2601.metamon.services.RemainingToday
import jp.kz2601.metamon.services.RemainingTodayStore
import jp.kz2601.metamon.services.http.FailureKind
import jp.kz2601.metamon.services.http.RateLimitInfo
import jp.kz2601.metamon.services.mock.MockSpeechTranscriber
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.Instant

/** 音声の画面の仕上げ(#892): 挨拶の出し分け・今日の残り・読み上げ・マイク FAB の吸着・頼み方のコツ。 */
@OptIn(ExperimentalCoroutinesApi::class)
class VoicePolishTest {

    // MARK: - 挨拶の出し分け

    @Test
    fun greetingContextFollowsVisitsAndEntry() {
        assertEquals(Greeting.Context.FIRST_VISIT, Greeting.context(Greeting.HomeEntry.LAUNCH, isInitial = true, launchIsReturning = false))
        assertEquals(Greeting.Context.RETURNING, Greeting.context(Greeting.HomeEntry.LAUNCH, isInitial = true, launchIsReturning = true))
        assertEquals(Greeting.Context.NEW_APP, Greeting.context(Greeting.HomeEntry.NEW_APP, isInitial = true, launchIsReturning = true))
        assertEquals(Greeting.Context.HOME, Greeting.context(Greeting.HomeEntry.HOME, isInitial = true, launchIsReturning = false))
        // 作ったアプリの上(FAB のシート)は経路に関わらず「どう変えますか?」
        assertEquals(Greeting.Context.EDITING, Greeting.context(Greeting.HomeEntry.NEW_APP, isInitial = false, launchIsReturning = true))
    }

    @Test
    fun greetingCopyPerContext() {
        assertEquals(Greeting.Copy(R.string.voice_title_first, R.string.voice_subtitle_first, true), Greeting.copy(Greeting.Context.FIRST_VISIT))
        assertEquals(Greeting.Copy(R.string.voice_title_returning, R.string.voice_subtitle_returning, true), Greeting.copy(Greeting.Context.RETURNING))
        // 新しいアプリ・改変は時間帯の挨拶を前置きしない(iOS と同じ)
        assertFalse(Greeting.copy(Greeting.Context.NEW_APP).subtitleHasHello)
        assertTrue(Greeting.copy(Greeting.Context.HOME).subtitleHasHello)
        assertEquals(R.string.voice_title_editing, Greeting.copy(Greeting.Context.EDITING).title)
        // 5 種類とも別の見出し
        assertEquals(5, Greeting.Context.entries.map { Greeting.copy(it).title }.toSet().size)
    }

    @Test
    fun firstLaunchIsNotReturning() {
        assertFalse(LaunchVisits.note(previousVisits = 0))
        assertTrue(LaunchVisits.note(previousVisits = 1))
    }

    @Test
    fun timeOfDayBoundaries() {
        assertEquals(R.string.voice_hello_evening, Greeting.timeOfDay(4))
        assertEquals(R.string.voice_hello_morning, Greeting.timeOfDay(5))
        assertEquals(R.string.voice_hello_morning, Greeting.timeOfDay(10))
        assertEquals(R.string.voice_hello_day, Greeting.timeOfDay(11))
        assertEquals(R.string.voice_hello_day, Greeting.timeOfDay(17))
        assertEquals(R.string.voice_hello_evening, Greeting.timeOfDay(18))
    }

    // MARK: - 今日の残り

    private val now = Instant.parse("2026-11-11T03:00:00Z")
    private val reset = Instant.parse("2026-11-11T15:00:00Z")
    private fun today(n: Int) = RemainingToday(n, 60, reset, observedAt = now)

    @Test
    fun remainingTodayDisplay() {
        assertEquals(RemainingTodayDisplay.Hidden, RemainingTodayDisplay.of(null, now, isPro = false))
        // 戻る時刻を過ぎた値は出さない
        assertEquals(RemainingTodayDisplay.Hidden, RemainingTodayDisplay.of(today(30), reset, isPro = false))
        assertEquals(RemainingTodayDisplay.Normal(30, pro = true), RemainingTodayDisplay.of(today(30), now, isPro = true))
        // 新しく作る 2 回分(6)以下は少ない
        assertEquals(RemainingTodayDisplay.Normal(7, pro = false), RemainingTodayDisplay.of(today(7), now, isPro = false))
        assertEquals(RemainingTodayDisplay.Low(6, pro = false), RemainingTodayDisplay.of(today(6), now, isPro = false))
        assertEquals(RemainingTodayDisplay.Low(1, pro = false), RemainingTodayDisplay.of(today(1), now, isPro = false))
        assertEquals(RemainingTodayDisplay.Exhausted(reset), RemainingTodayDisplay.of(today(0), now, isPro = false))
    }

    // MARK: - 読み上げ

    private class FakeEngine : SpeechEngine {
        val spoken = mutableListOf<Pair<String, String>>()
        var stops = 0
        override fun speak(text: String, languageTag: String) {
            spoken += text to languageTag
        }

        override fun stop() {
            stops++
        }
    }

    @Test
    fun speechFollowsSettingAndMic() {
        var on = false
        val engine = FakeEngine()
        var others = 0
        val reader = SpeechReader(enabled = { on }, engine = engine, stopOthers = { others++ })
        assertFalse(reader.speak("こんにちは"))
        assertTrue(engine.spoken.isEmpty())

        on = true
        assertTrue(reader.speak("  こんにちは  "))
        assertEquals("こんにちは" to "ja-JP", engine.spoken.last())
        assertTrue(reader.speak("Hello"))
        assertEquals("Hello" to "en-US", engine.spoken.last())
        assertFalse(reader.speak("   "))

        // マイクを押したら止めて、録音が終わるまで読まない(speakButton も止める)
        reader.stopForRecording()
        assertEquals(1, engine.stops)
        assertEquals(1, others)
        assertFalse(reader.speak("質問です"))
        reader.recordingFinished()
        assertTrue(reader.speak("質問です"))
        assertEquals(3, engine.spoken.size)
    }

    @Test
    fun languageFromText() {
        assertEquals("ja-JP", SpeechReader.language("家計簿"))
        assertEquals("ja-JP", SpeechReader.language("OK です"))
        assertEquals("en-US", SpeechReader.language("What do you want?"))
    }

    private fun doc(voiceOnly: Boolean) = AppDocument(
        appId = "app-1",
        version = 0,
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

    private class FakeAI(val turn: ChatTurn) : AIClient {
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
        ): MorphResult = MorphResult(document = current!!.copy(version = currentVersion + 1), summary = "家計簿を作りました", envelopeJson = "{}")

        override suspend fun chat(appId: String, messages: List<ChatMessage>, locale: String?, mode: ChatMode?): ChatTurn = turn
        override suspend fun resetChat(appId: String) = Unit
    }

    private class FakeUsage(var remaining: Int) : PlanUsageSource {
        var calls = 0
        override suspend fun myUsage(): MyUsage {
            calls++
            return MyUsage(plan = "free", resetAt = "2026-11-11T15:00:00Z", ai = MyUsage.Ai(used = 60 - remaining, limit = 60, remaining = remaining))
        }
    }

    private fun TestScope.vm(
        speechOn: Boolean,
        engine: FakeEngine,
        clarify: Boolean = true,
        usage: PlanUsageSource? = null,
        clock: () -> Instant = { now },
        ai: AIClient = FakeAI(ChatTurn(kind = ChatTurn.Kind.QUESTION, text = "何を記録しますか?", turn = 1)),
        voiceOnly: Boolean = true,
        remainingFeed: StateFlow<RemainingToday?>? = null,
    ): VoiceViewModel {
        val dispatcher = StandardTestDispatcher(testScheduler)
        return VoiceViewModel(
            transcriber = MockSpeechTranscriber(),
            ai = ai,
            appId = { "app-1" },
            initial = doc(voiceOnly),
            clarifyEnabled = { clarify },
            io = dispatcher,
            speech = SpeechReader(enabled = { speechOn }, engine = engine),
            usage = usage,
            clock = clock,
            remainingFeed = remainingFeed,
            scope = CoroutineScope(dispatcher + Job()),
        )
    }

    @Test
    fun readsAiQuestionWhenOn() = runTest {
        val engine = FakeEngine()
        val vm = vm(speechOn = true, engine = engine)
        advanceUntilIdle()
        vm.submit("家計簿")
        advanceUntilIdle()
        assertEquals(listOf("何を記録しますか?" to "ja-JP"), engine.spoken)
    }

    @Test
    fun doesNotReadWhenOff() = runTest {
        val engine = FakeEngine()
        val vm = vm(speechOn = false, engine = engine)
        advanceUntilIdle()
        assertFalse(vm.speakGreeting("どんなアプリが欲しいですか?"))
        vm.submit("家計簿")
        advanceUntilIdle()
        assertTrue(engine.spoken.isEmpty())
    }

    @Test
    fun readsSummaryAfterGenerate() = runTest {
        val engine = FakeEngine()
        val ai = FakeAI(ChatTurn(kind = ChatTurn.Kind.QUESTION, text = "?"))
        ai.current = doc(voiceOnly = false)
        val vm = vm(speechOn = true, engine = engine, clarify = false, ai = ai)
        advanceUntilIdle()
        vm.submit("家計簿")
        advanceUntilIdle()
        assertEquals("家計簿を作りました", engine.spoken.single().first)
    }

    @Test
    fun greetingReadOnlyOnInitialScreenWithoutChat() = runTest {
        val engine = FakeEngine()
        val initial = vm(speechOn = true, engine = engine)
        advanceUntilIdle()
        assertTrue(initial.speakGreeting("どんなアプリが欲しいですか?"))
        // 相談が始まったら読まない
        initial.submit("家計簿")
        advanceUntilIdle()
        assertFalse(initial.speakGreeting("どんなアプリが欲しいですか?"))
        // 作ったアプリの上(FAB のシート)では読まない
        val editing = vm(speechOn = true, engine = FakeEngine(), voiceOnly = false)
        advanceUntilIdle()
        assertFalse(editing.speakGreeting("このアプリをどう変えますか?"))
    }

    @Test
    fun micTapStopsSpeech() = runTest {
        val engine = FakeEngine()
        val vm = vm(speechOn = true, engine = engine)
        advanceUntilIdle()
        vm.onMicTap()
        advanceUntilIdle()
        assertEquals(1, engine.stops)
        // 録音中は質問が来ても読まない
        assertFalse(vm.speakGreeting("どんなアプリが欲しいですか?"))
    }

    @Test
    fun homeEntrySwitchesGreeting() = runTest {
        val vm = vm(speechOn = false, engine = FakeEngine())
        assertEquals(Greeting.HomeEntry.LAUNCH, vm.state.value.homeEntry)
        vm.noteHomeEntry(Greeting.HomeEntry.NEW_APP)
        assertEquals(Greeting.HomeEntry.NEW_APP, vm.state.value.homeEntry)
    }

    @Test
    fun remainingTodayFromUsageWithCache() = runTest {
        val usage = FakeUsage(remaining = 42)
        var clock = now
        val vm = vm(speechOn = false, engine = FakeEngine(), usage = usage, clock = { clock })
        assertNull(vm.state.value.remainingToday)
        vm.refreshUsage()
        advanceUntilIdle()
        assertEquals(42, vm.state.value.remainingToday?.remaining)
        assertEquals(RemainingTodayDisplay.Normal(42, pro = false), RemainingTodayDisplay.of(vm.state.value.remainingToday, now, vm.state.value.isPro))
        // 5 分は読み直さない
        usage.remaining = 40
        clock = now.plusSeconds(60)
        vm.refreshUsage()
        advanceUntilIdle()
        assertEquals(1, usage.calls)
        // 相談の往復の後は読み直す(force)
        vm.submit("家計簿")
        advanceUntilIdle()
        assertEquals(2, usage.calls)
        assertEquals(40, vm.state.value.remainingToday?.remaining)
    }

    @Test
    fun remainingTodayFromResponseHeadersWithoutUsage() = runTest {
        // #1203: usage を読み直さなくても、ai の応答のヘッダーで数が減る。古い値では上書きしない
        val store = RemainingTodayStore()
        val vm = vm(speechOn = false, engine = FakeEngine(), remainingFeed = store.value)
        advanceUntilIdle()
        val reset = Instant.parse("2026-11-11T15:00:00Z")
        store.record(RemainingToday(30, 60, reset, observedAt = now))
        advanceUntilIdle()
        assertEquals(30, vm.state.value.remainingToday?.remaining)
        store.record(RemainingToday(27, 60, reset, observedAt = now.plusSeconds(10)))
        advanceUntilIdle()
        assertEquals(27, vm.state.value.remainingToday?.remaining)
        assertNull(vm.state.value.error)
    }

    @Test
    fun exhaustedShowsLimitCardOncePerDay() = runTest {
        val store = RemainingTodayStore()
        val vm = vm(speechOn = false, engine = FakeEngine(), remainingFeed = store.value)
        advanceUntilIdle()
        val reset = Instant.parse("2026-11-11T15:00:00Z")
        store.record(RemainingToday(0, 60, reset, observedAt = now))
        advanceUntilIdle()
        val kind = (vm.state.value.error as? VoiceError.Request)?.kind as? FailureKind.RateLimited
        assertEquals(RateLimitInfo.Scope.USER, kind?.info?.scope)
        assertEquals(reset, kind?.info?.resetAt)
        assertEquals(0, kind?.info?.remaining)
        // 閉じたら同じ日にはもう出さない
        vm.dismissError()
        store.record(RemainingToday(0, 60, reset, observedAt = now.plusSeconds(10)))
        advanceUntilIdle()
        assertNull(vm.state.value.error)
        // 次の日の 0 はまた出す
        val next = reset.plusSeconds(86_400)
        store.record(RemainingToday(0, 60, next, observedAt = now.plusSeconds(20)))
        advanceUntilIdle()
        assertTrue(vm.state.value.error is VoiceError.Request)
    }

    // MARK: - マイク FAB の吸着

    @Test
    fun fabSnapsToNearestSideAndHidesNearEdge() {
        val w = 400f
        val size = 64f
        assertEquals(FabDock.Snap(FabSide.LEFT, hidden = false), FabDock.snap(100f, 100f, w, size))
        assertEquals(FabDock.Snap(FabSide.RIGHT, hidden = false), FabDock.snap(300f, 300f, w, size))
        // 弾いた方向(予測位置)で辺を選ぶ
        assertEquals(FabSide.RIGHT, FabDock.snap(150f, 260f, w, size).side)
        // 端から 64 × 0.3 = 19.2 以内(はみ出しを含む)で離すと隠す
        assertTrue(FabDock.snap(19f, 19f, w, size).hidden)
        assertFalse(FabDock.snap(20f, 20f, w, size).hidden)
        assertTrue(FabDock.snap(-10f, -10f, w, size).hidden)
        assertTrue(FabDock.snap(390f, 390f, w, size).hidden)
    }

    @Test
    fun fabRevealAndBottom() {
        assertTrue(FabDock.shouldReveal(20f, FabSide.LEFT, 20f))
        assertFalse(FabDock.shouldReveal(-30f, FabSide.LEFT, 20f))
        assertTrue(FabDock.shouldReveal(-25f, FabSide.RIGHT, 20f))
        // 自動は既定の下端距離。ユーザー値は既定〜上限に収める
        assertEquals(50f, FabDock.displayedBottom(FabDock.AUTO_BOTTOM, autoInset = 50f, maxBottom = 600f))
        assertEquals(50f, FabDock.displayedBottom(10f, autoInset = 50f, maxBottom = 600f))
        assertEquals(600f, FabDock.displayedBottom(900f, autoInset = 50f, maxBottom = 600f))
        assertEquals(300f, FabDock.storedBottom(centerY = 468f, height = 800f, fabSize = 64f, minBottom = 50f, maxBottom = 600f))
        assertEquals(50f, FabDock.storedBottom(centerY = 790f, height = 800f, fabSize = 64f, minBottom = 50f, maxBottom = 600f))
    }

    @Test
    fun fabHideHintOnlyFirstTime() {
        assertTrue(FabHideHint.shouldShow(wasHidden = false, nowHidden = true, alreadyShown = false))
        assertFalse(FabHideHint.shouldShow(wasHidden = false, nowHidden = true, alreadyShown = true))
        assertFalse(FabHideHint.shouldShow(wasHidden = true, nowHidden = true, alreadyShown = false))
        assertFalse(FabHideHint.shouldShow(wasHidden = false, nowHidden = false, alreadyShown = false))
    }

    // MARK: - 頼み方のコツ

    @Test
    fun promptTipsFilterByAvailableFeatures() {
        val all = PromptTips.categories()
        assertEquals(10, all.size)
        assertEquals(32, all.sumOf { it.tips.size })
        // 共有が無い版では共有のカテゴリごと消え、歩数(ヘルスケア)の例も出ない
        val noShare = PromptTips.categories(setOf(PromptTips.Feature.CAMERA_CAPTURE))
        assertTrue(noShare.none { it.id == "share" })
        assertTrue(noShare.flatMap { it.tips }.none { it.id == "stats_steps" || it.id == "map_route" })
        assertEquals(all.flatMap { it.tips }.map { it.id }.toSet().size, 32)
    }
}
