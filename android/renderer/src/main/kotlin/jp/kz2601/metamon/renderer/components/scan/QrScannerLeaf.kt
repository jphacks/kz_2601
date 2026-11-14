package jp.kz2601.metamon.renderer.components.scan

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.renderer.R
import jp.kz2601.metamon.renderer.components.actions.ActionButton
import jp.kz2601.metamon.renderer.components.actions.ActionButtonKind
import jp.kz2601.metamon.renderer.components.inputs.FormInput
import jp.kz2601.metamon.renderer.components.inputs.inputChrome
import jp.kz2601.metamon.renderer.form.setText
import jp.kz2601.metamon.renderer.form.text
import jp.kz2601.metamon.renderer.registry.RenderContext
import jp.kz2601.metamon.renderer.theme.RenderTheme

/**
 * `qrScanner`(iOS の `QrScannerLeaf`): 押すと QR / バーコードを読み、中身を文字列で form の field に入れる。
 * 読めない端末(Play 開発者サービスが無い・読み取りの画面を出せなかった)は手入力の欄だけ(iOS のシミュレータと同じ)。
 */
@Composable
fun QrScannerLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) = FormInput(c, ctx, modifier) { form, field ->
    val context = LocalContext.current
    val engine = LocalQrScanEngine.current
    val available = remember(engine, context) { engine.isAvailable(context) }
    var failed by rememberSaveable { mutableStateOf(false) }
    var scanning by remember { mutableStateOf(false) }
    val label = c.label ?: stringResource(R.string.renderer_scan_qr)

    Column(Modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.xs)) {
        if (available && !failed) {
            ActionButton(
                label = label,
                kind = ActionButtonKind.SECONDARY,
                running = scanning,
                onClick = {
                    scanning = true
                    engine.scan(context) { result ->
                        scanning = false
                        when (val o = QrScanRules.outcome(result)) {
                            is QrScanOutcome.Write -> form.values[field] = o.value
                            QrScanOutcome.ManualOnly -> failed = true
                            QrScanOutcome.Keep -> Unit
                        }
                    }
                },
            )
            QrScanRules.preview(form.values[field])?.let {
                Text(
                    it,
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    maxLines = 2,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.semantics { liveRegion = LiveRegionMode.Polite },
                )
            }
        } else {
            Row(Modifier.fillMaxWidth().inputChrome(), verticalAlignment = Alignment.CenterVertically) {
                Text(label, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.weight(1f))
                Text(stringResource(R.string.renderer_scan_unavailable), style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            OutlinedTextField(
                value = form.text(field),
                onValueChange = { form.setText(field, it) },
                modifier = Modifier.fillMaxWidth(),
                placeholder = { Text(stringResource(R.string.renderer_scan_manual)) },
                singleLine = true,
                isError = form.isFlagged(field),
                shape = RoundedCornerShape(RenderTheme.Radius.control),
            )
        }
    }
}
