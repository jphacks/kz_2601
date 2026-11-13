package jp.kz2601.metamon.desktop.automations

import kotlinx.serialization.Serializable
import java.time.Instant

/** 運営の停止が効いている印(`GET /v1/apps/{appId}/automations` の `stopped`)。 */
@Serializable
data class AutomationStop(val until: String? = null)

/** ルール 1 本(openapi `AutomationRule` のうち「直しますか?」に要る分)。日時は ISO 8601 の文字のまま持つ。 */
@Serializable
data class AutomationRule(
    val id: String,
    val description: String = "",
    val lastOkAt: String? = null,
    /** 運営がこのルールを止めている */
    val stopped: AutomationStop? = null,
    /** 作成者のオン / オフ。null = 返さない古いサーバー(オンとして扱う) */
    val enabled: Boolean? = null,
    val consecutiveFailures: Int = 0,
) {
    /** 何回も続けて失敗している(「直しますか?」を出す) */
    val suggestsFix: Boolean get() = consecutiveFailures >= FIX_SUGGESTION_THRESHOLD

    companion object {
        const val FIX_SUGGESTION_THRESHOLD = 3
    }
}

/** アプリの自動の動きの一覧(`GET /v1/apps/{appId}/automations`)。 */
@Serializable
data class AppAutomations(
    val appId: String = "",
    /** 運営がアプリの自動の動きを全部止めている */
    val stopped: AutomationStop? = null,
    val rules: List<AutomationRule> = emptyList(),
)

/**
 * 修正の入力を開いた時の「直しますか?」を出すかの規則(iOS の `AutomationFixOffer`)。描画はしない。
 */
object AutomationFixOffer {
    /**
     * 出すルール(1 本だけ)。運営が止めている・作成者がオフ・閉じた / 直した連続失敗は出さない。
     * 複数あれば失敗の続いた回数が多い方、同じなら一覧の順。
     */
    fun pick(automations: AppAutomations, handled: Set<String>): AutomationRule? {
        if (automations.stopped != null) return null
        return automations.rules
            .filter { it.suggestsFix && it.stopped == null && it.enabled != false && key(automations.appId, it) !in handled }
            .withIndex()
            .maxWithOrNull(compareBy<IndexedValue<AutomationRule>> { it.value.consecutiveFailures }.thenByDescending { it.index })
            ?.value
    }

    /** 連続失敗 1 回分の印。成功すると連続が 0 に戻り lastOkAt が進むので、次の連続では別の印になる。 */
    fun key(appId: String, rule: AutomationRule): String {
        val since = rule.lastOkAt?.let { seconds(it) } ?: "never"
        return "$appId|${rule.id}|$since"
    }

    /** 読めない日時は文字のまま印にする(ずれても別の連続として扱うだけ)。 */
    private fun seconds(iso: String): String =
        runCatching { Instant.parse(iso).epochSecond.toString() }.getOrDefault(iso)
}
