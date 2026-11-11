package jp.kz2601.metamon.update

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.expandVertically
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.shrinkVertically
import androidx.compose.foundation.clickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.consumeWindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawingPadding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBars
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.Info
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import jp.kz2601.metamon.R
import jp.kz2601.metamon.language.AppLocales
import jp.kz2601.metamon.services.update.ServiceNotice

/**
 * 起動時の確認と、お知らせのバナー・更新の画面(iOS の `UpdateGateModifier`)。サインイン前の画面も含めて全体を包む。
 * 前面に戻った時の読み直しは AppForeground(ForegroundSteps の "update")が持つ。
 */
@Composable
fun UpdateGateHost(gate: UpdateGate, content: @Composable () -> Unit) {
    val context = LocalContext.current
    val state by gate.state.collectAsStateWithLifecycle()
    LaunchedEffect(gate) { gate.check() }
    val notice = gate.visibleNotice(state)

    Box(Modifier.fillMaxSize()) {
        Column(Modifier.fillMaxSize()) {
            AnimatedVisibility(notice != null, enter = expandVertically() + fadeIn(), exit = shrinkVertically() + fadeOut()) {
                val shown = notice ?: return@AnimatedVisibility
                ServiceNoticeBanner(
                    shown,
                    onOpenLink = { url -> openLink(context, url) },
                    onDismiss = gate::dismissNotice,
                )
            }
            // バナーが状態バーの分を取るので、下の画面には状態バーの余白を足させない
            val inner = if (notice != null) Modifier.consumeWindowInsets(WindowInsets.statusBars) else Modifier
            Box(inner.fillMaxWidth().weight(1f)) { content() }
        }
        if (state.isBlocking) UpdateRequiredScreen(onOpenStore = { PlayStore.open(context, state.storeUrl) })
    }

    if (state.showsRecommended) {
        AlertDialog(
            onDismissRequest = gate::dismissRecommended,
            title = { Text(stringResource(R.string.update_recommended_title)) },
            text = { Text(stringResource(R.string.update_recommended_body)) },
            confirmButton = {
                TextButton(onClick = {
                    gate.dismissRecommended()
                    PlayStore.open(context, state.storeUrl)
                }) { Text(stringResource(R.string.update_update)) }
            },
            dismissButton = { TextButton(onClick = gate::dismissRecommended) { Text(stringResource(R.string.update_later)) } },
            modifier = Modifier.testTag("update.recommended"),
        )
    }
}

/** アプリ内のお知らせのバナー(#333)。1 行の告知・任意の「詳しく見る」・閉じるボタン。 */
@Composable
fun ServiceNoticeBanner(notice: ServiceNotice, onOpenLink: (String) -> Unit, onDismiss: () -> Unit) {
    val context = LocalContext.current
    val text = remember(notice) { notice.text(AppLocales.languageCode(context)) }
    val warning = notice.severity == ServiceNotice.Severity.WARNING
    val tint = if (warning) WARNING_TINT else MaterialTheme.colorScheme.primary
    val label = stringResource(if (warning) R.string.notice_a11y_warning else R.string.notice_a11y_info, text)
    Surface(color = MaterialTheme.colorScheme.surfaceContainerHigh, modifier = Modifier.fillMaxWidth().testTag("notice.banner")) {
        Column(Modifier.statusBarsPadding()) {
            Row(Modifier.padding(start = 16.dp, end = 4.dp, top = 8.dp, bottom = 8.dp), verticalAlignment = Alignment.Top) {
                Icon(
                    if (warning) Icons.Filled.Warning else Icons.Filled.Info,
                    contentDescription = null,
                    tint = tint,
                    modifier = Modifier.padding(top = 12.dp).size(20.dp),
                )
                Column(
                    Modifier.weight(1f).padding(start = 12.dp, top = 10.dp)
                        .semantics {
                            contentDescription = label
                            liveRegion = LiveRegionMode.Polite
                        },
                ) {
                    Text(text, style = MaterialTheme.typography.bodyMedium)
                    notice.link?.let { url ->
                        TextButton(onClick = { onOpenLink(url) }, modifier = Modifier.testTag("notice.link")) {
                            Text(stringResource(R.string.notice_learn_more), fontWeight = FontWeight.SemiBold)
                        }
                    }
                }
                // タップ領域は 48dp(IconButton の既定)
                IconButton(onClick = onDismiss, modifier = Modifier.testTag("notice.dismiss")) {
                    Icon(Icons.Filled.Close, contentDescription = stringResource(R.string.notice_dismiss))
                }
            }
            HorizontalDivider(color = tint.copy(alpha = 0.6f))
        }
    }
}

/** min 未満の版で出す、閉じられない画面。下の画面には触らせない。 */
@Composable
private fun UpdateRequiredScreen(onOpenStore: () -> Unit) {
    Surface(
        color = MaterialTheme.colorScheme.background,
        modifier = Modifier.fillMaxSize().testTag("update.required")
            // 下の画面へのタップを止める
            .clickable(interactionSource = remember { MutableInteractionSource() }, indication = null, onClick = {}),
    ) {
        Column(
            Modifier.fillMaxSize().safeDrawingPadding().padding(32.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.spacedBy(20.dp),
        ) {
            Spacer(Modifier.weight(1f))
            Text(stringResource(R.string.update_required_title), style = MaterialTheme.typography.headlineSmall, fontWeight = FontWeight.Bold)
            Text(
                stringResource(R.string.update_required_body),
                textAlign = TextAlign.Center,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Spacer(Modifier.weight(1f))
            Button(onClick = onOpenStore, modifier = Modifier.fillMaxWidth().height(52.dp).testTag("update.openStore")) {
                Text(stringResource(R.string.update_open_store))
            }
        }
    }
}

/** iOS の .orange に当たる色(ライト・ダークどちらでも読める濃さ) */
private val WARNING_TINT = Color(0xFFF57C00)
