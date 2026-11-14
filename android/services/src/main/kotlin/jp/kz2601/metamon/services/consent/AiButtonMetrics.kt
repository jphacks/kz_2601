package jp.kz2601.metamon.services.consent

import jp.kz2601.metamon.services.http.ApiError
import jp.kz2601.metamon.services.http.ApiJson
import jp.kz2601.metamon.services.http.HttpRequest
import jp.kz2601.metamon.services.http.MetamonHttp
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.serialization.Serializable
import kotlinx.serialization.builtins.MapSerializer
import kotlinx.serialization.builtins.serializer
import java.time.Instant
import java.time.ZoneId
import java.util.UUID

/**
 * `POST /v1/metrics/ai-button` の本文(iOS の `AIButtonMetricsReport`・#888)。決まったラベルと回数だけ。
 * ラベルは backend の `aibutton_metrics.go` と同じ(snake_case)。
 */
@Serializable
data class AiButtonMetricsReport(val days: List<Day>) {
    @Serializable
    data class TaskCount(val task: String, val attempts: Int, val failures: Map<String, Int>)

    @Serializable
    data class Day(
        /** 日ごとの乱数(再送の重複を消すため。端末・人とは結ばない) */
        val reportId: String,
        /** 端末のローカル日(YYYY-MM-DD) */
        val day: String,
        val availability: String,
        val tasks: List<TaskCount>,
    )
}

fun interface AiButtonMetricsSender {
    suspend fun send(report: AiButtonMetricsReport)
}

/** appconfig の `POST /v1/metrics/ai-button`(iOS の `HTTPAIButtonMetricsSender`)。 */
class HttpAiButtonMetricsSender(private val http: MetamonHttp) : AiButtonMetricsSender {
    override suspend fun send(report: AiButtonMetricsReport) {
        http.send(
            HttpRequest(
                "POST",
                listOf("v1", "metrics", "ai-button"),
                body = ApiJson.encodeToString(AiButtonMetricsReport.serializer(), report),
                timeoutMillis = 20_000,
            ),
        )
    }
}

/** backend が受け付けるラベル。Renderer のラベル(camelCase)をここで揃え、知らない値は other にする。 */
object AiButtonMetricLabels {
    val availabilities = setOf("available", "device_not_eligible", "apple_intelligence_off", "model_not_ready", "os_too_old", "other")
    val tasks = setOf("summarize", "classify", "rewrite", "extract", "suggest", "generate", "other")
    val reasons = setOf(
        "unavailable", "invalid_response", "context_too_long", "guardrail", "unsupported_language",
        "refusal", "rate_limited", "assets_unavailable", "model_error", "bad_config",
    )

    fun availability(raw: String): String = snake(raw).takeIf { it in availabilities } ?: "other"

    /** DSL の task(未知の値は other・DSL の文字列をそのまま送らない) */
    fun task(raw: String?): String = raw?.let(::snake)?.takeIf { it in tasks } ?: "other"

    fun reason(raw: String): String = snake(raw).takeIf { it in reasons } ?: "model_error"

    private fun snake(raw: String): String = raw.replace(Regex("([a-z0-9])([A-Z])"), "$1_$2").lowercase()
}

/**
 * aiButton の端末内 AI の集計(iOS の `AIButtonMetricsRecorder`・#888)。端末のローカル日ごとに数を貯め、
 * 前の日までの分を 1 日 1 回まとめて送る。設定の「AI ボタンの利用状況を送る」でオフにすると、貯めた分も消して数えない。
 * uid・利用者の文・記録は持たない。
 */
class AiButtonMetricsRecorder(
    private val store: KeyValueStore,
    private val clock: () -> Instant = Instant::now,
    private val zone: () -> ZoneId = ZoneId::systemDefault,
    private val newReportId: () -> String = { UUID.randomUUID().toString() },
) {
    @Serializable
    internal data class Bucket(
        val reportId: String,
        val availability: String? = null,
        val attempts: Map<String, Int> = emptyMap(),
        val failures: Map<String, Map<String, Int>> = emptyMap(),
    )

    private val _enabled = MutableStateFlow(store.getBoolean(ENABLED_KEY) ?: true)
    val enabled: StateFlow<Boolean> = _enabled.asStateFlow()
    val isEnabled: Boolean get() = _enabled.value

    private val lock = Any()
    private var buckets: Map<String, Bucket> = load()
    private var flushing = false

    fun setEnabled(enabled: Boolean) {
        store.putBoolean(ENABLED_KEY, enabled)
        _enabled.value = enabled
        if (!enabled) synchronized(lock) {
            buckets = emptyMap()
            save()
        }
    }

    /** 貯めた数だけ消す(サインアウト・削除・#841)。オン / オフの選択は本人の設定なので残す(iOS の `preservedKeys`)。 */
    fun clearCounts() = synchronized(lock) {
        buckets = emptyMap()
        save()
    }

    /** ボタンを出した時の可用性(その日の最後の値を送る)。 */
    fun recordShown(availability: String) = update { b ->
        val a = AiButtonMetricLabels.availability(availability)
        if (b.availability == a) null else b.copy(availability = a) // 表示のたびに書かない
    }

    /** 端末内 AI で 1 回動かした。failure = null は答えを出せた。 */
    fun recordRan(task: String?, failure: String?) = update { b ->
        val t = AiButtonMetricLabels.task(task)
        val failures = failure?.let { AiButtonMetricLabels.reason(it) }?.let { r ->
            val forTask = b.failures[t].orEmpty()
            b.failures + (t to forTask + (r to (forTask[r] ?: 0) + 1))
        } ?: b.failures
        b.copy(
            availability = b.availability ?: "available", // 端末内で動いた = その時は使えた
            attempts = b.attempts + (t to (b.attempts[t] ?: 0) + 1),
            failures = failures,
        )
    }

    /** 今日より前の日の分(新しい順に最大 [KEEP_DAYS] 日)。無ければ null。 */
    fun pendingReport(): AiButtonMetricsReport? {
        val today = today()
        val snapshot = synchronized(lock) { buckets }
        val days = snapshot.keys.filter { it < today }.sorted().takeLast(KEEP_DAYS).mapNotNull { key ->
            val b = snapshot.getValue(key)
            val availability = b.availability ?: return@mapNotNull null
            val tasks = b.attempts.keys.sorted().map { t ->
                AiButtonMetricsReport.TaskCount(t, b.attempts[t] ?: 0, b.failures[t].orEmpty())
            }
            AiButtonMetricsReport.Day(b.reportId, key, availability, tasks)
        }
        return if (days.isEmpty()) null else AiButtonMetricsReport(days)
    }

    /**
     * 前の日までの分を送る(起動・前面に戻った時)。送れたら消す。400(形の誤り)も消す。通信の失敗は次回に回す。
     * 送るのは前の日までの分だけなので、同じ日に何度呼んでも送るのは 1 日 1 回。
     */
    suspend fun flush(sender: AiButtonMetricsSender) {
        prune()
        val report = synchronized(lock) {
            if (!isEnabled || flushing) return
            pendingReport()?.also { flushing = true }
        } ?: return
        try {
            sender.send(report)
            drop(report)
        } catch (e: CancellationException) {
            throw e
        } catch (e: ApiError) {
            if (e.status == 400) drop(report)
        } catch (_: Exception) {
            // 圏外など: 次の起動・前面で送り直す
        } finally {
            synchronized(lock) { flushing = false }
        }
    }

    private fun update(change: (Bucket) -> Bucket?) {
        if (!isEnabled) return
        synchronized(lock) {
            val key = today()
            val current = buckets[key] ?: Bucket(reportId = newReportId())
            val next = change(current) ?: return
            buckets = buckets + (key to next)
            save()
        }
    }

    private fun drop(report: AiButtonMetricsReport) = synchronized(lock) {
        buckets = buckets - report.days.map { it.day }.toSet()
        save()
    }

    /** [KEEP_DAYS] より古い日を捨てる(圏外が続いても増え続けない)。 */
    private fun prune() = synchronized(lock) {
        val cutoff = clock().atZone(zone()).toLocalDate().minusDays(KEEP_DAYS.toLong()).toString()
        val kept = buckets.filterKeys { it >= cutoff }
        if (kept.size != buckets.size) {
            buckets = kept
            save()
        }
    }

    private fun today(): String = clock().atZone(zone()).toLocalDate().toString()

    private fun load(): Map<String, Bucket> = store.getString(BUCKETS_KEY)
        ?.let { runCatching { ApiJson.decodeFromString(bucketsSerializer, it) }.getOrNull() }
        .orEmpty()

    private fun save() {
        if (buckets.isEmpty()) store.remove(BUCKETS_KEY) else store.putString(BUCKETS_KEY, ApiJson.encodeToString(bucketsSerializer, buckets))
    }

    companion object {
        const val ENABLED_KEY = "metamon.aiButtonMetrics.enabled"
        const val BUCKETS_KEY = "metamon.aiButtonMetrics.buckets"

        /** 送らずに貯めておく日数の上限 */
        const val KEEP_DAYS = 7

        private val bucketsSerializer = MapSerializer(String.serializer(), Bucket.serializer())
    }
}
