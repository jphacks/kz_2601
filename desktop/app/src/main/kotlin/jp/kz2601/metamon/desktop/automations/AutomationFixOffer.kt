package jp.kz2601.metamon.desktop.automations

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.services.automations.AppAutomations
import jp.kz2601.metamon.services.automations.AutomationRule
import jp.kz2601.metamon.services.automations.AutomationsClient
import jp.kz2601.metamon.services.consent.KeyValueStore
import kotlinx.coroutines.CancellationException
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonPrimitive

/**
 * 「このアプリを直す」を開いた時の「直しますか?」(iOS の `AutomationFixOffer`・#3356)。
 * 続けて失敗している自動の動きがあれば、直す依頼を入力欄に入れられる(送るのは本人)。
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
            .maxWithOrNull(compareBy<AutomationRule> { it.consecutiveFailures }.thenByDescending { automations.rules.indexOf(it) })
    }

    /** 連続失敗 1 回分の印。成功すると lastOkAt が進むので、次の連続では別の印になる。 */
    fun key(appId: String, rule: AutomationRule): String = "$appId|${rule.id}|${rule.lastOkAt ?: "never"}"

    /** 入力欄の上の 1 行 */
    fun message(rule: AutomationRule): String = Strings.format("desktop_automation_fix_offer", rule.label)

    /** 押すと入力欄に入る依頼(Android の自動の動きの一覧の「直しますか?」と同じ文) */
    fun request(rule: AutomationRule): String = Strings.format("automations_fix_request", rule.label)
}

/** 閉じた / 直した印(設定の置き場に JSON の配列で)。古い印から捨てる。 */
class AutomationFixOfferStore(private val store: KeyValueStore) {
    fun handled(): Set<String> = list().toSet()

    fun markHandled(key: String) {
        val list = list()
        if (key in list) return
        val kept = (list + key).takeLast(LIMIT)
        store.putString(KEY, JsonArray(kept.map(::JsonPrimitive)).toString())
    }

    private fun list(): List<String> = runCatching {
        Json.parseToJsonElement(store.getString(KEY) ?: return emptyList()).jsonArray.mapNotNull { it.jsonPrimitive.contentOrNull }
    }.getOrDefault(emptyList())

    companion object {
        const val KEY = "tailorit.automationFixOffer.handled.v1"
        const val LIMIT = 200
    }
}

/** 直す入力を開いた時に 1 回読み、出すルールを持つ。読めない(圏外・作成者でない 403)時は何も出さない。 */
class AutomationFixOfferModel(
    val appId: String,
    private val client: AutomationsClient,
    private val store: AutomationFixOfferStore,
) {
    var rule: AutomationRule? by mutableStateOf(null)
        private set

    suspend fun load() {
        val a = try {
            client.automations(appId)
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
            return
        }
        rule = AutomationFixOffer.pick(a, store.handled())
    }

    /** 「直す」: 印を付け、入力欄に入れる依頼を返す(送らない)。 */
    fun fix(): String? {
        val r = rule ?: return null
        store.markHandled(AutomationFixOffer.key(appId, r))
        rule = null
        return AutomationFixOffer.request(r)
    }

    /** 閉じる: 同じ連続失敗ではもう出さない。 */
    fun dismiss() {
        val r = rule ?: return
        store.markHandled(AutomationFixOffer.key(appId, r))
        rule = null
    }
}

/**
 * 直す入力の上に出す 1 行。「直す」は [onFill] で依頼を入力欄に入れる(送らない)。
 * 直す入力(#2164)ができたら、その入力欄の上に置き、開いた時に [AutomationFixOfferModel.load] を呼ぶ。
 */
@Composable
fun AutomationFixOfferRow(model: AutomationFixOfferModel, onFill: (String) -> Unit, modifier: Modifier = Modifier) {
    val rule = model.rule ?: return
    Surface(color = MaterialTheme.colorScheme.surfaceVariant, shape = MaterialTheme.shapes.medium, modifier = modifier.fillMaxWidth()) {
        Row(
            Modifier.padding(horizontal = 12.dp, vertical = 6.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            Text(
                AutomationFixOffer.message(rule),
                style = MaterialTheme.typography.bodyMedium,
                modifier = Modifier.weight(1f).semantics { liveRegion = LiveRegionMode.Polite },
            )
            Button(onClick = { model.fix()?.let(onFill) }) { Text(Strings["desktop_automation_fix_offer_fix"]) }
            TextButton(onClick = model::dismiss) { Text(Strings["desktop_close"]) }
        }
    }
}
