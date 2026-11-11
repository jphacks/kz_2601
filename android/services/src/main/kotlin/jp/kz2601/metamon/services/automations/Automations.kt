package jp.kz2601.metamon.services.automations

import jp.kz2601.metamon.services.http.ApiError
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.intOrNull
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.put
import jp.kz2601.metamon.services.http.ApiJson

/**
 * アプリの「自動の動き」(serverRules)の一覧と直近の結果(`GET /v1/apps/{appId}/automations`・#347 / #905)。作成者だけが読める。
 * iOS の `AppAutomations`。日時はサーバーの ISO 8601 の文字のまま持つ(表示の時に読む)。
 */
@Serializable
data class AppAutomations(
    val appId: String = "",
    /** 運営がアプリの自動の動きを全部止めている */
    val stopped: AutomationStop? = null,
    /** 自動の動きは動くが、通知は止めている */
    val notifyPaused: AutomationStop? = null,
    val quota: AutomationQuota? = null,
    val rules: List<AutomationRule> = emptyList(),
)

/** 運営の停止が効いている印。[until] は自動で解ける時刻(null = 運営が解くまで)。 */
@Serializable
data class AutomationStop(val until: String? = null)

/** アプリの今日の実行の数(日本時間の 0 時で戻る)。 */
@Serializable
data class AutomationQuota(val used: Int, val limit: Int, val resetAt: String? = null)

/** ルール 1 本(openapi `AutomationRule`)。 */
@Serializable
data class AutomationRule(
    val id: String,
    /** サーバーが作る説明 */
    val description: String = "",
    /** "record" | "schedule" */
    val trigger: String = "record",
    @SerialName("result") val resultWire: String? = null,
    val lastRunAt: String? = null,
    val lastOkAt: String? = null,
    /** 運営がこのルールを止めている */
    val stopped: AutomationStop? = null,
    /** 作成者のオン / オフ。null = 返さない古いサーバー(切り替えを出さない) */
    val enabled: Boolean? = null,
    val consecutiveFailures: Int = 0,
    val failureReason: String? = null,
) {
    val label: String get() = description.ifEmpty { id }
    val isSchedule: Boolean get() = trigger == "schedule"
    val result: AutomationResult get() = AutomationResult.of(resultWire)

    /** 何回も続けて失敗している(「直しますか?」を出す) */
    val suggestsFix: Boolean get() = consecutiveFailures >= FIX_SUGGESTION_THRESHOLD

    companion object {
        const val FIX_SUGGESTION_THRESHOLD = 3
    }
}

/** 直近の結果(openapi `AutomationResult`)。知らない語は [OTHER]。 */
enum class AutomationResult(val wire: String) {
    NONE("none"), OK("ok"), NOTIFY_PAUSED("notify_paused"), LIMIT("limit"), SERVICE_LIMIT("service_limit"),
    SERVICE_PAUSED("service_paused"), PLAN("plan"), STOPPED("stopped"), CHANGED("changed"), UNAVAILABLE("unavailable"),
    LATE("late"), INACTIVE("inactive"), FAILED("failed"), DISABLED("disabled"), OTHER("other");

    /** 止まった・動かなかった。作成者がオフにした([DISABLED])は含めない */
    val isProblem: Boolean get() = this != NONE && this != OK && this != DISABLED

    companion object {
        fun of(wire: String?): AutomationResult = if (wire == null) NONE else entries.firstOrNull { it.wire == wire } ?: OTHER
    }
}

/** 自動の動きの見える化(iOS の `AutomationsClient`)。 */
interface AutomationsClient {
    /** 作成者以外は 403([ApiError.status])。 */
    suspend fun automations(appId: String): AppAutomations

    /** ルールのオン / オフ(`PATCH /v1/apps/{appId}/dsl`・If-Match)。他で変わっていれば 412。 */
    suspend fun setRule(ruleId: String, enabled: Boolean, appId: String)
}

/**
 * ルールのオン / オフを `PATCH /v1/apps/{appId}/dsl` の JSON Patch にする(iOS の `AutomationToggle`)。
 * オフ = `enabled: false` を足す、オン = `enabled` を外す(省略 = オン)。
 */
object AutomationToggle {
    sealed class Failure(message: String) : Exception(message) {
        /** 今の DSL にそのルールが無い(一覧が古い) */
        class RuleNotFound : Failure("rule not found")

        /** `GET /dsl` の応答が読めない */
        class BadEnvelope : Failure("bad dsl envelope")
    }

    data class Current(val version: Int, val serverRules: List<JsonObject>)

    /** 今の DSL(`GET /dsl` の封筒)から version と serverRules を取り出す。 */
    fun parse(envelope: String): Current {
        val obj = runCatching { ApiJson.parseToJsonElement(envelope).jsonObject }.getOrNull() ?: throw Failure.BadEnvelope()
        val version = (obj["version"] as? JsonPrimitive)?.intOrNull ?: throw Failure.BadEnvelope()
        val rules = ((obj["dsl"] as? JsonObject)?.get("serverRules") as? JsonArray)?.mapNotNull { it as? JsonObject }.orEmpty()
        return Current(version, rules)
    }

    /** id で位置を引いて ops を作る(一覧の並びに頼らない)。もうその状態なら空(PATCH しない)。 */
    fun ops(ruleId: String, enabled: Boolean, serverRules: List<JsonObject>): List<JsonObject> {
        val i = serverRules.indexOfFirst { (it["id"] as? JsonPrimitive)?.contentOrNull == ruleId }
        if (i < 0) throw Failure.RuleNotFound()
        val path = "/serverRules/$i/enabled"
        val current = serverRules[i]["enabled"]
        if (enabled) {
            return if (current == null) emptyList() else listOf(buildJsonObject { put("op", "remove"); put("path", path) })
        }
        return if ((current as? JsonPrimitive)?.booleanOrNull == false) {
            emptyList()
        } else {
            listOf(buildJsonObject { put("op", "add"); put("path", path); put("value", false) })
        }
    }

    /** 他で変わっていた(取り直して見せる): 412 / 409 / 404・ルールが見つからない。 */
    fun isConflict(error: Throwable): Boolean = when (error) {
        is Failure.RuleNotFound -> true
        is ApiError -> error.status in setOf(404, 409, 412)
        else -> false
    }
}

/** 通信しない構成(`METAMON_MOCK`)の自動の動き。アプリごとに見本の 2 本をメモリに持つ。 */
class MockAutomationsClient(private val seed: List<AutomationRule> = DEMO) : AutomationsClient {
    private val apps = mutableMapOf<String, List<AutomationRule>>()

    override suspend fun automations(appId: String): AppAutomations = synchronized(apps) {
        AppAutomations(appId = appId, quota = AutomationQuota(used = 2, limit = 100), rules = apps.getOrPut(appId) { seed })
    }

    override suspend fun setRule(ruleId: String, enabled: Boolean, appId: String) = synchronized(apps) {
        val rules = apps.getOrPut(appId) { seed }
        if (rules.none { it.id == ruleId }) throw AutomationToggle.Failure.RuleNotFound()
        apps[appId] = rules.map { if (it.id == ruleId) it.copy(enabled = enabled) else it }
    }

    companion object {
        val DEMO = listOf(
            AutomationRule(id = "notify-new", description = "「記録」が作られた時、アプリの作成者に通知", resultWire = "ok", enabled = true),
            AutomationRule(id = "daily-summary", description = "毎日 9:00 に、今日の予定をまとめて通知", trigger = "schedule", enabled = false),
        )
    }
}
