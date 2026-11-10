package jp.kz2601.metamon.renderer.components.ai

import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.Length
import jp.kz2601.metamon.dsl.Task
import jp.kz2601.metamon.dsl.ai.AiSaveTo
import jp.kz2601.metamon.dsl.ai.AiTextError
import jp.kz2601.metamon.dsl.ai.AiTextResult
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.HiddenFields
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.renderer.DSLRuntime
import jp.kz2601.metamon.renderer.form.FormDraft
import kotlinx.coroutines.CancellationException

/**
 * aiButton の 1 回分の実行(iOS の AiButtonLeaf / AiGenerateLeaf / AiSaveToList の run・save)。画面の状態は持たない。
 * 結果の保存先: 要約などは output のドラフト、generate は output のドラフト(フォームがある時)とボタンの下、
 * saveTo は押した行だけを [DSLRuntime.addRecord] で 1 件ずつ足す。
 */
object AiButtonRun {
    fun route(c: Component, runtime: DSLRuntime): AiRoute {
        val host = runtime.ai
        return AiRoute.choose(
            availability = (host?.onDevice ?: UnsupportedOnDeviceAi).availability(),
            hostOnline = host?.supportsOnlineText ?: false,
            mock = host?.isMock ?: false,
            long = c.length == Length.LONG,
            photo = c.inputPhoto != null,
        )
    }

    /** 要約・分類など: 結果を output のドラフトに入れる。オンラインで書いたら true。 */
    suspend fun transform(c: Component, form: FormDraft, runtime: DSLRuntime, locale: String, route: AiRoute = route(c, runtime)): Boolean {
        val output = c.output ?: throw AiBuildError(AiBuildError.Kind.UNKNOWN_TASK)
        if (route is AiRoute.Unavailable) throw AiTextError.Unsupported
        val task = c.task?.wire ?: ""
        return metered(runtime, route, task) {
            val (source, records) = inputRecords(c, runtime, route)
            val values = formValues(form, runtime)
            if (route == AiRoute.OnDevice) {
                val request = AiRequestBuilder.build(c, form.entity, values, source, records)
                val raw = runtime.ai!!.onDevice.run(request)
                form.values[output] = JSONValue.Str(AiRequestBuilder.normalize(raw, request) ?: throw AiTextError.BadOutput)
                false
            } else {
                val host = runtime.ai ?: throw AiTextError.Unsupported
                val request = AiRequestBuilder.online(c, form.entity, values, source, records, locale)
                form.values[output] = JSONValue.Str(host.generateText(request).text)
                true
            }
        }
    }

    /** generate: 結果を返す。フォームがあり output があればドラフトにも入れる。 */
    suspend fun generate(c: Component, form: FormDraft?, runtime: DSLRuntime, locale: String, route: AiRoute = route(c, runtime)): AiTextResult {
        if (route is AiRoute.Unavailable) throw AiTextError.Unsupported
        return metered(runtime, route, Task.GENERATE.wire) {
            val (source, records) = inputRecords(c, runtime, route)
            val values = form?.let { formValues(it, runtime) }.orEmpty()
            val request = AiGenerateBuilder.build(c, form?.entity, values, source, records, locale)
            val host = runtime.ai ?: throw AiTextError.Unsupported
            val out = if (route == AiRoute.OnDevice) host.onDevice.generate(request) else host.generateText(request)
            val output = c.output
            if (output != null && form != null) form.values[output] = JSONValue.Str(out.text)
            out
        }
    }

    /** 保存の結果。[saved] / [failed] は items の位置。 */
    data class SaveOutcome(val saved: Set<Int>, val failed: Set<Int>, val full: Boolean) {
        val added: Int get() = saved.size
        val notAdded: Int get() = failed.size
    }

    /** saveTo: 押した行だけを entity の記録として足す(上限を超える分は足さない)。 */
    suspend fun save(saveTo: AiSaveTo, items: List<String>, indices: List<Int>, runtime: DSLRuntime): SaveOutcome {
        val entity = runtime.entityModel(saveTo.entity) ?: return SaveOutcome(emptySet(), indices.toSet(), full = false)
        runtime.loadRecords(setOf(entity.id))
        val existing = runtime.recordsFlow?.value?.get(entity.id).orEmpty().size
        val room = AiSaveTo.room(existing, indices.size)
        val saved = mutableSetOf<Int>()
        val failed = mutableSetOf<Int>()
        for ((n, i) in indices.withIndex()) {
            val data = if (n < room) items.getOrNull(i)?.let { saveTo.record(it, entity) } else null
            if (data != null && runtime.addRecord(entity.id, data)) saved += i else failed += i
        }
        return SaveOutcome(saved, failed, full = room < indices.size && saved.isEmpty())
    }

    /** `entity` 入力の記録: hidden を落とし、クラウドへは自分の記録だけ。 */
    internal suspend fun inputRecords(c: Component, runtime: DSLRuntime, route: AiRoute): Pair<Entity?, List<Record>> {
        val id = c.entity ?: return null to emptyList()
        val e = runtime.entityModel(id) ?: return null to emptyList()
        runtime.loadRecords(setOf(id))
        val visible = HiddenFields.redact(runtime.recordsFlow?.value?.get(id).orEmpty(), e) // 回答者に見せない項目は渡さない(#262)
        return e to AiInputRecords.select(visible, e, route, runtime.currentUid())
    }

    private fun formValues(form: FormDraft, runtime: DSLRuntime): Map<String, JSONValue> =
        if (form.showsHiddenFields(runtime.currentUid())) form.values.toMap() else HiddenFields.redact(form.values.toMap(), form.entity)

    /** 端末内 AI の時だけ成否を数える(#446・入力待ちは数えない)。 */
    private suspend fun <T> metered(runtime: DSLRuntime, route: AiRoute, task: String, body: suspend () -> T): T {
        try {
            val out = body()
            if (route == AiRoute.OnDevice) runtime.ai?.record(AiButtonMetric.Ran(task, null))
            return out
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            if (route == AiRoute.OnDevice) failureLabel(e)?.let { runtime.ai?.record(AiButtonMetric.Ran(task, it)) }
            throw e
        }
    }

    internal fun failureLabel(e: Exception): String? = when {
        e is AiBuildError && e.isWaitingForInput -> null
        e is AiBuildError -> "badConfig"
        e is AiTextError.BadOutput -> "invalidResponse"
        e is AiTextError.Unsupported -> "unavailable"
        else -> "modelError"
    }
}
