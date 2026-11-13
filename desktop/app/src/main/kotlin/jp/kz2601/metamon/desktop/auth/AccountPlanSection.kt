package jp.kz2601.metamon.desktop.auth

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.desktop.net.AccountPlan
import jp.kz2601.metamon.desktop.net.AccountPlanLoader
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter

/** アカウントの画面の「プラン」「使っている量」の文(#2205)。値段・上限の数はサーバーが返した物だけを出す。 */
object AccountPlanText {
    /** 右の値: Pro / 無料 / 読めませんでした。 */
    fun value(plan: AccountPlan, lang: Lang = Strings.lang): String = when (plan.kind) {
        AccountPlan.Kind.PRO -> "Pro"
        AccountPlan.Kind.FREE -> Strings.table.get(lang, "settings_pro_free")
        AccountPlan.Kind.UNKNOWN -> Strings.table.get(lang, "desktop_plan_unknown")
    }

    /** 下の説明の行(期限・どこで買ったか・前回の値か)。 */
    fun details(plan: AccountPlan, lang: Lang = Strings.lang, zone: ZoneId = ZoneId.systemDefault()): List<String> = buildList {
        val date = plan.until?.let { date(it, lang, zone) }
        when {
            plan.kind == AccountPlan.Kind.PRO && date != null -> add(Strings.table.format(lang, "desktop_plan_until", date))
            plan.expired && date != null -> add(Strings.table.format(lang, "desktop_plan_expired", date))
        }
        if (plan.kind == AccountPlan.Kind.PRO) {
            when (plan.store) {
                AccountPlan.PurchaseStore.APP_STORE -> add(Strings.table.get(lang, "desktop_plan_store_app_store"))
                AccountPlan.PurchaseStore.GOOGLE_PLAY -> add(Strings.table.get(lang, "desktop_plan_store_google_play"))
                null -> Unit
            }
        }
        if (plan.cached) add(Strings.table.get(lang, "desktop_plan_cached"))
    }

    /** 作ったアプリの数(`12 / 50`・上限が無ければ「12(上限なし)」)。読めなければ null。 */
    fun apps(plan: AccountPlan, lang: Lang = Strings.lang): String? {
        val m = plan.usage?.apps ?: return null
        return m.limit?.let { "${m.used} / $it" } ?: Strings.table.format(lang, "settings_usage_no_limit", m.used)
    }

    fun date(at: Instant, lang: Lang = Strings.lang, zone: ZoneId = ZoneId.systemDefault()): String =
        DateTimeFormatter.ofPattern(Strings.table.get(lang, "desktop_plan_date_pattern"), lang.locale).withZone(zone).format(at)
}

/**
 * アカウントの画面に置く「プラン」と「使っている量」(#2205)。サインイン中([uid] がある時)だけ読む。
 * 買う所は作らない(#1760 で決める)。
 */
@Composable
fun AccountPlanSection(loader: AccountPlanLoader, uid: String?, modifier: Modifier = Modifier) {
    uid ?: return
    var plan by remember(uid) { mutableStateOf<AccountPlan?>(null) }
    LaunchedEffect(uid) { plan = loader.load(uid) }
    val p = plan ?: return
    Column(modifier) {
        Row(Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 8.dp).semantics(mergeDescendants = true) {}) {
            Text(Strings["settings_pro_plan"], modifier = Modifier.weight(1f))
            Text(
                AccountPlanText.value(p),
                fontWeight = if (p.kind == AccountPlan.Kind.PRO) FontWeight.SemiBold else null,
                color = if (p.kind == AccountPlan.Kind.PRO) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        AccountPlanText.details(p).forEach {
            Text(
                it,
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(horizontal = 16.dp, vertical = 2.dp),
            )
        }
        AccountPlanText.apps(p)?.let { apps ->
            Row(Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 8.dp).semantics(mergeDescendants = true) {}) {
                Text(Strings["settings_usage_apps"], modifier = Modifier.weight(1f))
                Text(apps, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        }
    }
}
