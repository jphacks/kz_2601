package jp.kz2601.metamon.renderer

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.Length
import jp.kz2601.metamon.dsl.Shape
import jp.kz2601.metamon.dsl.Task
import jp.kz2601.metamon.dsl.ai.AiSaveTo
import jp.kz2601.metamon.dsl.ai.AiTextError
import jp.kz2601.metamon.dsl.ai.AiTextRequest
import jp.kz2601.metamon.dsl.ai.AiTextResult
import jp.kz2601.metamon.dsl.ai.AiTextShape
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.dsl.records.entityModels
import jp.kz2601.metamon.renderer.components.ai.AiBuildError
import jp.kz2601.metamon.renderer.components.ai.AiButtonMetric
import jp.kz2601.metamon.renderer.components.ai.AiButtonRun
import jp.kz2601.metamon.renderer.components.ai.AiCopy
import jp.kz2601.metamon.renderer.components.ai.AiGenerateBuilder
import jp.kz2601.metamon.renderer.components.ai.AiHost
import jp.kz2601.metamon.renderer.components.ai.AiInputRecords
import jp.kz2601.metamon.renderer.components.ai.AiOnDeviceRequest
import jp.kz2601.metamon.renderer.components.ai.AiRequestBuilder
import jp.kz2601.metamon.renderer.components.ai.AiRoute
import jp.kz2601.metamon.renderer.components.ai.OnDeviceAi
import jp.kz2601.metamon.renderer.components.ai.OnDeviceAvailability
import jp.kz2601.metamon.renderer.form.FormDraft
import jp.kz2601.metamon.renderer.registry.ComponentRegistry
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Test

/** aiButton(#873): どこで動かすか・入力の作り方・結果の保存先・エラーの分け方(iOS の AiButtonTests / AiButtonOnlineTests)。 */
class AiButtonTest {
    private val dsl = AppDSL.decode(
        """
        {"schemaVersion":1,"app":{"name":"t","voiceEntry":"none"},"initialScreen":"form",
         "entities":[
          {"id":"diary","name":"日記","fields":[
            {"id":"body","name":"本文","type":"text"},
            {"id":"mood","name":"気分","type":"enum","options":["嬉しい","悲しい"]},
            {"id":"secret","name":"秘密","type":"string","hidden":true},
            {"id":"photo","name":"写真","type":"photo"},
            {"id":"summary","name":"要約","type":"string"}]},
          {"id":"post","name":"投稿","scope":"shared","fields":[{"id":"text","name":"文","type":"string"}]},
          {"id":"idea","name":"案","fields":[{"id":"title","name":"案","type":"string"},{"id":"done","name":"済み","type":"boolean"}]}],
         "screens":[{"id":"form","title":"t","entity":"diary","components":[]}]}
        """.trimIndent(),
    )
    private val entities = dsl.entityModels()
    private val diary: Entity = entities.first { it.id == "diary" }
    private val post: Entity = entities.first { it.id == "post" }

    private fun ai(task: Task, output: String? = null, input: String? = null, entity: String? = null, prompt: String? = null, limit: Int? = null) =
        Component(type = ComponentType.AI_BUTTON, id = "ai", task = task, output = output, input = input, entity = entity, prompt = prompt, limit = limit)

    // MARK: どこで動かすか

    @Test
    fun routePrefersOnDeviceThenOnlineThenReason() {
        val ok = OnDeviceAvailability.Available
        val no = OnDeviceAvailability.Unavailable(OnDeviceAvailability.Reason.DEVICE_NOT_ELIGIBLE)
        assertEquals(AiRoute.OnDevice, AiRoute.choose(ok, hostOnline = true, mock = false))
        assertEquals(AiRoute.Online, AiRoute.choose(no, hostOnline = true, mock = false))
        assertEquals(AiRoute.Online, AiRoute.choose(ok, hostOnline = true, mock = true))
        assertEquals(AiRoute.Unavailable(AiRoute.UnavailableReason.Device(OnDeviceAvailability.Reason.DEVICE_NOT_ELIGIBLE)), AiRoute.choose(no, hostOnline = false, mock = false))
        assertEquals(AiRoute.Online, AiRoute.choose(ok, hostOnline = true, mock = false, long = true))
        assertEquals(AiRoute.Unavailable(AiRoute.UnavailableReason.LongOnlyOnline), AiRoute.choose(ok, hostOnline = false, mock = false, long = true))
        assertEquals(AiRoute.Unavailable(AiRoute.UnavailableReason.PhotoOnlyOnline), AiRoute.choose(ok, hostOnline = false, mock = false, photo = true))
    }

    @Test
    fun noHostMeansUnavailableOnThisDevice() {
        val route = AiButtonRun.route(ai(Task.SUMMARIZE, output = "summary"), NoopDSLRuntime)
        assertTrue(route is AiRoute.Unavailable)
    }

    // MARK: 入力の作り方

    @Test
    fun formInputSkipsOutputAndMediaFields() {
        val values = mapOf("body" to JSONValue.Str("散歩した"), "mood" to JSONValue.Str("嬉しい"), "summary" to JSONValue.Str("古い要約"))
        val text = AiRequestBuilder.inputText(ai(Task.SUMMARIZE, output = "summary"), diary, values, null, emptyList())
        assertEquals("本文: 散歩した\n気分: 嬉しい", text)
        assertEquals("散歩した", AiRequestBuilder.inputText(ai(Task.SUMMARIZE, output = "summary", input = "body"), diary, values, null, emptyList()))
    }

    @Test
    fun onDeviceClassifyListsOptionsAndAppendsInputWhenTemplateLacksIt() {
        val c = ai(Task.CLASSIFY, output = "mood", input = "body", prompt = "気分を選んで")
        val r = AiRequestBuilder.build(c, diary, mapOf("body" to JSONValue.Str("楽しかった")), null, emptyList())
        assertEquals(listOf("嬉しい", "悲しい"), r.options)
        assertEquals("気分を選んで\n\n楽しかった\n\n候補: 嬉しい / 悲しい", r.prompt)
        assertEquals("嬉しい", AiRequestBuilder.normalize("「たぶん嬉しい」", r))
        assertNull(AiRequestBuilder.normalize("怒り", r))
    }

    @Test
    fun onlineRequestUsesMarkerAndSendsInputSeparately() {
        val c = ai(Task.SUMMARIZE, output = "summary", input = "body", prompt = "{{input}} を 1 行で")
        val r = AiRequestBuilder.online(c, diary, mapOf("body" to JSONValue.Str("長い日記")), locale = "ja")
        assertEquals("下の材料 を 1 行で", r.instruction)
        assertEquals("長い日記", r.input)
        assertEquals("summarize", r.task)
        assertEquals(AiTextShape.LINE, r.shape)
        assertEquals("the data below を 1 行で", AiRequestBuilder.online(c, diary, mapOf("body" to JSONValue.Str("x")), locale = "en").instruction)
    }

    @Test
    fun buildErrorsAreSeparatedFromWaitingForInput() {
        val empty = runCatching { AiRequestBuilder.online(ai(Task.SUMMARIZE, output = "summary", input = "body"), diary, emptyMap(), locale = "ja") }.exceptionOrNull() as AiBuildError
        assertEquals(AiBuildError.Kind.EMPTY_INPUT, empty.kind)
        assertTrue(empty.isWaitingForInput)
        val noTask = runCatching { AiRequestBuilder.build(ai(Task.GENERATE, output = "summary"), diary, emptyMap()) }.exceptionOrNull() as AiBuildError
        assertEquals(AiBuildError.Kind.UNKNOWN_TASK, noTask.kind)
        assertFalse(noTask.isWaitingForInput)
        val noInstruction = runCatching { AiGenerateBuilder.build(ai(Task.GENERATE), null, emptyMap(), null, emptyList(), "ja") }.exceptionOrNull() as AiBuildError
        assertEquals(AiBuildError.Kind.NO_INSTRUCTION, noInstruction.kind)
    }

    @Test
    fun generateUsesRecentRecordsUpToLimit() {
        val records = (1..5).map { Record("r$it", mapOf("body" to JSONValue.Str("日記$it")), createdAt = "2026-11-0${it}T00:00:00Z") }
        val c = Component(type = ComponentType.AI_BUTTON, task = Task.GENERATE, prompt = "励まして", entity = "diary", limit = 2, shape = Shape.LIST, count = 4)
        val r = AiGenerateBuilder.build(c, null, emptyMap(), diary, records, "ja")
        assertEquals("- 本文: 日記5\n- 本文: 日記4", r.input)
        assertEquals(4, r.count)
        assertEquals(AiTextShape.LIST, r.shape)
        val noRecords = runCatching { AiGenerateBuilder.build(c, null, emptyMap(), diary, emptyList(), "ja") }.exceptionOrNull() as AiBuildError
        assertEquals(AiBuildError.Kind.NO_RECORDS, noRecords.kind)
        val long = AiGenerateBuilder.build(c.copy(length = Length.LONG), null, emptyMap(), diary, records, "ja")
        assertEquals("long", long.length)
    }

    @Test
    fun cloudGetsOnlyOwnRecordsOfSharedEntities() {
        val records = listOf(
            Record("a", author = "me", updatedAt = "2026-11-02"),
            Record("b", author = "other", updatedAt = "2026-11-03"),
            Record("c", unsent = true, updatedAt = "2026-11-01"),
            Record("d", updatedAt = "2026-11-04"), // 作者なし・送信済み(サーバーの規則が作った分など)
        )
        assertEquals(listOf("a", "c"), AiInputRecords.select(records, post, AiRoute.Online, "me").map { it.id })
        assertEquals(4, AiInputRecords.select(records, post, AiRoute.OnDevice, "me").size) // 端末内は絞らない
        assertEquals(4, AiInputRecords.select(records, diary, AiRoute.Online, "me").size) // private は全部自分
        val e = runCatching { AiInputRecords.select(listOf(records[1]), post, AiRoute.Online, "me") }.exceptionOrNull() as AiBuildError
        assertEquals(AiBuildError.Kind.NO_OWN_RECORDS, e.kind)
    }

    // MARK: 結果の保存先

    private class FakeOnDevice(val available: Boolean, val answer: String = "嬉しい") : OnDeviceAi {
        var ran = 0
        override fun availability() = if (available) OnDeviceAvailability.Available else OnDeviceAvailability.Unavailable(OnDeviceAvailability.Reason.MODEL_NOT_READY)
        override suspend fun run(request: AiOnDeviceRequest): String = answer.also { ran++ }
        override suspend fun generate(request: AiTextRequest): AiTextResult = AiTextResult(AiTextShape.LINE, "端末の答え")
    }

    private class FakeHost(
        override val onDevice: OnDeviceAi,
        override val supportsOnlineText: Boolean = true,
        val answer: (AiTextRequest) -> AiTextResult = { AiTextResult(AiTextShape.LINE, "オンラインの答え") },
    ) : AiHost {
        val sent = mutableListOf<AiTextRequest>()
        val metrics = mutableListOf<AiButtonMetric>()
        override suspend fun generateText(request: AiTextRequest): AiTextResult = answer(request).also { sent += request }
        override fun record(event: AiButtonMetric) {
            metrics += event
        }
    }

    private inner class FakeRuntime(override val ai: AiHost?, records: Map<String, List<Record>> = emptyMap(), private val uid: String? = "me") : DSLRuntime {
        val flow = MutableStateFlow(records)
        val added = mutableListOf<Pair<String, Map<String, JSONValue>>>()
        val loaded = mutableListOf<Set<String>>()
        override fun onVoiceInput(componentId: String?) = Unit
        override val entityModels: List<Entity> get() = entities
        override val recordsFlow: StateFlow<Map<String, List<Record>>> get() = flow
        override fun currentUid(): String? = uid
        override suspend fun loadRecords(entities: Set<String>) {
            loaded += entities
        }
        override suspend fun addRecord(entity: String, data: Map<String, JSONValue>): Boolean {
            added += entity to data
            flow.value = flow.value + (entity to flow.value[entity].orEmpty() + Record("n${added.size}", data))
            return true
        }
    }

    @Test
    fun transformOnDeviceFillsOutputDraftAndCountsRun() = runBlocking<Unit> {
        val host = FakeHost(FakeOnDevice(available = true))
        val form = FormDraft(diary).apply { values["body"] = JSONValue.Str("楽しかった") }
        val online = AiButtonRun.transform(ai(Task.CLASSIFY, output = "mood", input = "body"), form, FakeRuntime(host), "ja")
        assertFalse(online)
        assertEquals(JSONValue.Str("嬉しい"), form.values["mood"])
        assertTrue(host.sent.isEmpty())
        assertEquals(AiButtonMetric.Ran("classify", null), host.metrics.single())
    }

    @Test
    fun transformOnlineWhenOnDeviceIsNotReadyAndHidesHiddenFieldsOfOthers() = runBlocking<Unit> {
        val host = FakeHost(FakeOnDevice(available = false))
        val form = FormDraft(diary, Record("r1", mapOf("body" to JSONValue.Str("本文"), "secret" to JSONValue.Str("答え")), author = "other"))
        val online = AiButtonRun.transform(ai(Task.SUMMARIZE, output = "summary"), form, FakeRuntime(host), "ja")
        assertTrue(online)
        assertEquals(JSONValue.Str("オンラインの答え"), form.values["summary"])
        val input = host.sent.single().input!!
        assertTrue(input.contains("本文"))
        assertFalse("他人の記録の hidden は渡さない", input.contains("答え"))
        assertTrue("オンラインは数えない", host.metrics.isEmpty())
    }

    @Test
    fun generateFillsOutputOnlyWithFormAndLeavesDraftOnFailure() = runBlocking<Unit> {
        val host = FakeHost(FakeOnDevice(available = false))
        val rt = FakeRuntime(host)
        val c = Component(type = ComponentType.AI_BUTTON, task = Task.GENERATE, prompt = "ひとこと", output = "summary")
        val form = FormDraft(diary)
        assertEquals("オンラインの答え", AiButtonRun.generate(c, form, rt, "ja").text)
        assertEquals(JSONValue.Str("オンラインの答え"), form.values["summary"])
        assertEquals("オンラインの答え", AiButtonRun.generate(c, null, rt, "ja").text) // フォームの外でも結果は返す

        val refusing = FakeHost(FakeOnDevice(available = false)) { throw AiTextError.NeedsPro }
        val untouched = FormDraft(diary)
        val before = untouched.values["summary"]
        try {
            AiButtonRun.generate(c, untouched, FakeRuntime(refusing), "ja")
            fail("NeedsPro が出るはず")
        } catch (_: AiTextError.NeedsPro) {
        }
        assertEquals(before, untouched.values["summary"])
    }

    @Test
    fun saveToAddsOnlyPressedRowsAsRecords() = runBlocking<Unit> {
        val rt = FakeRuntime(FakeHost(FakeOnDevice(available = false)), records = mapOf("idea" to emptyList()))
        val saveTo = AiSaveTo("idea", "title")
        val items = listOf("案A", "案B", "案C")
        val one = AiButtonRun.save(saveTo, items, listOf(1), rt)
        assertEquals(setOf(1), one.saved)
        assertEquals(listOf("idea" to mapOf("title" to JSONValue.Str("案B"), "done" to JSONValue.Bool(false))), rt.added)
        val rest = AiButtonRun.save(saveTo, items, listOf(0, 2), rt)
        assertEquals(2, rest.added)
        assertEquals(0, rest.notAdded)
        val bad = AiButtonRun.save(AiSaveTo("idea", "done"), items, listOf(0), rt)
        assertEquals(setOf(0), bad.failed)
        assertEquals(setOf(0), AiButtonRun.save(AiSaveTo("nope", "title"), items, listOf(0), rt).failed)
    }

    @Test
    fun entityInputLoadsRecordsBeforeBuilding() = runBlocking<Unit> {
        val records = listOf(Record("a", mapOf("text" to JSONValue.Str("投稿")), author = "me"))
        val host = FakeHost(FakeOnDevice(available = false))
        val rt = FakeRuntime(host, records = mapOf("post" to records))
        val c = Component(type = ComponentType.AI_BUTTON, task = Task.GENERATE, prompt = "まとめて", entity = "post")
        AiButtonRun.generate(c, null, rt, "ja")
        assertEquals(listOf(setOf("post")), rt.loaded)
        assertEquals("- 文: 投稿", host.sent.single().input)
    }

    // MARK: エラーの分け方

    @Test
    fun errorsPickTheirOwnMessages() {
        assertEquals(R.string.ai_error_needs_pro, AiCopy.error(AiTextError.NeedsPro).res)
        assertEquals(R.string.ai_error_creator_needs_pro, AiCopy.error(AiTextError.CreatorNeedsPro).res)
        assertEquals(R.string.ai_error_needs_consent, AiCopy.error(AiTextError.NeedsConsent).res)
        assertEquals(R.string.ai_error_paused, AiCopy.error(AiTextError.Paused).res)
        assertEquals(R.string.ai_error_no_own_records, AiCopy.error(AiBuildError(AiBuildError.Kind.NO_OWN_RECORDS)).res)
        assertEquals("今日の上限です", AiCopy.error(AiTextError.Failed("今日の上限です")).raw)
        assertEquals("boom", AiCopy.error(IllegalStateException("boom")).arg)
        assertEquals(R.string.ai_reason_long_only_online, AiCopy.reason(AiRoute.UnavailableReason.LongOnlyOnline))
        assertEquals(R.string.ai_label_summarize, AiCopy.label(Task.SUMMARIZE))
        assertEquals(R.string.ai_label_default, AiCopy.label(null))
    }

    @Test
    fun aiButtonIsRegistered() {
        assertTrue(ComponentType.AI_BUTTON.wire in ComponentRegistry.standard.registeredTypes)
    }
}
