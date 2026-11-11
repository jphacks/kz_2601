package jp.kz2601.metamon.purchase

import android.content.Intent
import android.net.Uri
import androidx.activity.compose.LocalActivity
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedCard
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import jp.kz2601.metamon.R
import jp.kz2601.metamon.consent.LegalLinks
import jp.kz2601.metamon.services.http.PaidFeature
import jp.kz2601.metamon.services.purchase.ProProducts
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

/**
 * 課金画面([ProStore.requestPaywall] で出す)と、サインインの後に Pro を読み直すところ。本体の画面に 1 つだけ置く。
 * [uid] が変わるたび(サインイン・人の切り替え)にサーバーの Pro を読み、送れていない Play の購入を送る。
 */
@Composable
fun PaywallHost(pro: ProStore, uid: String?) {
    LaunchedEffect(uid) { if (uid != null) pro.refresh() }
    val state by pro.state.collectAsStateWithLifecycle()
    val reason = state.paywall ?: return
    PaywallSheet(pro, reason, onDismiss = pro::dismissPaywall)
}

/**
 * 「Tailor It Pro」の課金画面(iOS の `PaywallSheet`・#77 / #414)。何ができるようになるか・価格・購入・復元・定期購入の管理・規約とプライバシー。
 * Play のポリシー: 定期購入の名前・期間・価格をボタンの外にも出し、自動更新と解約の方法を書く。
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun PaywallSheet(pro: ProStore, reason: PaywallReason, onDismiss: () -> Unit) {
    val state by pro.state.collectAsStateWithLifecycle()
    val activity = LocalActivity.current
    val scope = rememberCoroutineScope()
    LaunchedEffect(Unit) { pro.loadOffer() }
    // 買えたら少し見せてから閉じる(iOS と同じ)
    LaunchedEffect(state.isPro) {
        if (state.isPro && reason != PaywallReason.SETTINGS) {
            delay(1_400)
            onDismiss()
        }
    }
    ModalBottomSheet(
        onDismissRequest = onDismiss,
        sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true),
        modifier = Modifier.testTag("paywall.sheet"),
    ) {
        Column(
            Modifier.verticalScroll(rememberScrollState()).padding(horizontal = 24.dp).padding(bottom = 32.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            Text("Tailor It Pro", style = MaterialTheme.typography.headlineMedium.copy(fontWeight = FontWeight.Bold))
            Text(
                PaywallCopy.headlineText(reason),
                style = MaterialTheme.typography.titleMedium,
                textAlign = TextAlign.Center,
                modifier = Modifier.testTag("paywall.headline"),
            )
            PaywallCopy.reasonLines(reason).forEach {
                Text(
                    it,
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.testTag("paywall.reason"),
                )
            }
            if (state.isPro) {
                ProBadge(justPurchased = reason != PaywallReason.SETTINGS)
                ManageSubscriptionButton()
            } else {
                PlanRow(state)
                Button(
                    onClick = { activity?.let { a -> scope.launch { pro.purchase(a) } } },
                    enabled = !state.busy && state.offer != null && activity != null,
                    modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp).testTag("paywall.purchase"),
                ) {
                    if (state.busy) CircularProgressIndicator(Modifier.size(18.dp), strokeWidth = 2.dp)
                    Text(
                        state.offer?.let { stringResource(R.string.paywall_purchase_price, it.displayPrice) } ?: stringResource(R.string.paywall_purchase),
                        modifier = Modifier.padding(start = 8.dp),
                    )
                }
                TextButton(onClick = { scope.launch { pro.restore() } }, enabled = !state.busy, modifier = Modifier.testTag("paywall.restore")) {
                    Text(stringResource(R.string.paywall_restore))
                }
                ManageSubscriptionButton()
                if (state.offer == null && !state.loadingOffer) {
                    Text(stringResource(R.string.paywall_price_unavailable), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
            }
            state.message?.let {
                Text(
                    stringResource(PaywallCopy.message(it)),
                    style = MaterialTheme.typography.bodySmall,
                    textAlign = TextAlign.Center,
                    modifier = Modifier.testTag("paywall.message"),
                )
            }
            PaywallCopy.closeNote(reason)?.takeIf { !state.isPro }?.let {
                Text(stringResource(it), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant, textAlign = TextAlign.Center)
            }
            Benefits()
            Text(
                stringResource(R.string.paywall_fine_print),
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                textAlign = TextAlign.Center,
            )
            LegalLinks()
        }
    }
}

@Composable
private fun PlanRow(state: ProUiState) {
    OutlinedCard(Modifier.fillMaxWidth()) {
        Row(Modifier.padding(16.dp), verticalAlignment = Alignment.CenterVertically) {
            Column(Modifier.weight(1f)) {
                Text(state.offer?.displayName ?: "Tailor It Pro", style = MaterialTheme.typography.titleMedium)
                Text(stringResource(R.string.paywall_plan_period), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            when {
                state.offer != null -> Text(
                    stringResource(R.string.paywall_price_per_month, state.offer.displayPrice),
                    style = MaterialTheme.typography.titleLarge.copy(fontWeight = FontWeight.Bold),
                    modifier = Modifier.testTag("paywall.price"),
                )
                state.loadingOffer -> CircularProgressIndicator(Modifier.size(20.dp), strokeWidth = 2.dp)
            }
        }
    }
}

@Composable
private fun ProBadge(justPurchased: Boolean) {
    Text(
        stringResource(if (justPurchased) R.string.paywall_pro_now else R.string.paywall_pro_active),
        style = MaterialTheme.typography.titleMedium.copy(fontWeight = FontWeight.SemiBold),
        color = MaterialTheme.colorScheme.primary,
        modifier = Modifier.testTag("paywall.pro"),
    )
}

@Composable
private fun Benefits() {
    OutlinedCard(Modifier.fillMaxWidth().testTag("paywall.benefits")) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text(stringResource(R.string.paywall_benefits_title), style = MaterialTheme.typography.labelLarge, color = MaterialTheme.colorScheme.onSurfaceVariant)
            listOf(
                R.string.paywall_benefit_shared,
                R.string.paywall_benefit_join,
                R.string.paywall_benefit_unlimited,
                R.string.paywall_benefit_ai,
                R.string.paywall_benefit_edits,
                R.string.paywall_benefit_cancel,
            ).forEach { Text("・" + stringResource(it), style = MaterialTheme.typography.bodyMedium) }
        }
    }
}

/** Play の定期購入の管理の画面(解約・支払い方法)。iOS の `ManageSubscriptionButton` */
@Composable
fun ManageSubscriptionButton() {
    val context = LocalContext.current
    TextButton(onClick = {
        val url = "https://play.google.com/store/account/subscriptions?sku=${ProProducts.MONTHLY}&package=${context.packageName}"
        runCatching { context.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(url)).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)) }
    }, modifier = Modifier.testTag("paywall.manage")) {
        Text(stringResource(R.string.paywall_manage))
    }
}

/** 課金画面の文言の選び方(iOS の `PaywallSheet.headline` / `AITextPaywallCopy`) */
object PaywallCopy {
    fun headline(reason: PaywallReason): Int = when (reason) {
        PaywallReason.SETTINGS -> R.string.paywall_headline_settings
        PaywallReason.AI_TEXT -> R.string.paywall_headline_ai_text
        PaywallReason.AI_CHAT -> R.string.paywall_headline_ai_chat
        PaywallReason.LIMIT_REACHED -> R.string.paywall_headline_limit_reached
        is PaywallReason.PaidFeatures -> R.string.paywall_headline_paid_feature
    }

    @Composable
    fun headlineText(reason: PaywallReason): String = when (reason) {
        PaywallReason.LIMIT_REACHED -> stringResource(headline(reason), ProProducts.DAILY_FREE_CREATIONS)
        is PaywallReason.PaidFeatures -> stringResource(headline(reason), featureNames(reason.features) ?: stringResource(R.string.paywall_shared_data))
        else -> stringResource(headline(reason))
    }

    /** カードと同じ機能名(backend の title)。名前が無ければ null(呼び出し側で「みんなで共有するデータ」) */
    fun featureNames(features: List<PaidFeature>): String? =
        features.map { it.title }.filter { it.isNotEmpty() }.takeIf { it.isNotEmpty() }?.joinToString("・")

    /** 有料機能の理由(backend の reason)。無ければ共有のデータの費用の説明 */
    fun featureReasons(features: List<PaidFeature>): List<String> = features.map { it.reason }.filter { it.isNotEmpty() }

    @Composable
    fun reasonLines(reason: PaywallReason): List<String> = when (reason) {
        is PaywallReason.PaidFeatures -> featureReasons(reason.features).ifEmpty { listOf(stringResource(R.string.paywall_reason_shared_data)) }
        else -> listOfNotNull(this.reason(reason)?.let { stringResource(it) })
    }

    fun reason(reason: PaywallReason): Int? = when (reason) {
        PaywallReason.AI_TEXT -> R.string.paywall_reason_ai_text
        PaywallReason.AI_CHAT -> R.string.paywall_reason_ai_chat
        PaywallReason.SETTINGS, PaywallReason.LIMIT_REACHED, is PaywallReason.PaidFeatures -> null
    }

    fun closeNote(reason: PaywallReason): Int? = when (reason) {
        PaywallReason.AI_TEXT -> R.string.paywall_close_ai_text
        PaywallReason.AI_CHAT -> R.string.paywall_close_ai_chat
        PaywallReason.SETTINGS, PaywallReason.LIMIT_REACHED, is PaywallReason.PaidFeatures -> null
    }

    fun message(m: ProMessage): Int = when (m) {
        ProMessage.PENDING -> R.string.paywall_message_pending
        ProMessage.PURCHASE_FAILED -> R.string.paywall_message_failed
        ProMessage.NOT_VERIFIED -> R.string.paywall_message_not_verified
        ProMessage.UNAVAILABLE -> R.string.paywall_message_unavailable
        ProMessage.RESTORED -> R.string.paywall_message_restored
        ProMessage.RESTORE_NONE -> R.string.paywall_message_restore_none
        ProMessage.RESTORE_FAILED -> R.string.paywall_message_restore_failed
    }
}

/**
 * 設定の「Tailor It Pro」(iOS の `ProSettingsSection`): プラン・「Pro にする」・「購入を復元」・定期購入の管理。
 * Play も、モックの購入も無い構成では出さない。
 */
@Composable
fun ProSettingsSection(pro: ProStore, serverPro: Boolean) {
    if (!pro.isAvailable) return
    val state by pro.state.collectAsStateWithLifecycle()
    val isPro = state.isPro || serverPro
    val scope = rememberCoroutineScope()
    Column(Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 8.dp).testTag("settings.pro")) {
        Text("Tailor It Pro", style = MaterialTheme.typography.titleSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
        Row(Modifier.fillMaxWidth().padding(vertical = 8.dp), verticalAlignment = Alignment.CenterVertically) {
            Text(stringResource(R.string.settings_pro_plan), modifier = Modifier.weight(1f))
            Text(
                if (isPro) "Pro" else stringResource(R.string.settings_pro_free),
                color = if (isPro) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.onSurfaceVariant,
                fontWeight = if (isPro) FontWeight.SemiBold else FontWeight.Normal,
                modifier = Modifier.testTag("settings.pro.plan"),
            )
        }
        if (!isPro) {
            TextButton(onClick = { pro.requestPaywall(PaywallReason.SETTINGS) }, modifier = Modifier.testTag("settings.pro.upgrade")) {
                Text(stringResource(R.string.settings_pro_upgrade))
            }
        }
        TextButton(onClick = { scope.launch { pro.restore() } }, enabled = !state.busy, modifier = Modifier.testTag("settings.pro.restore")) {
            Text(stringResource(R.string.paywall_restore))
        }
        ManageSubscriptionButton()
        if (state.paywall == null) {
            state.message?.let { Text(stringResource(PaywallCopy.message(it)), style = MaterialTheme.typography.bodySmall) }
        }
    }
}
