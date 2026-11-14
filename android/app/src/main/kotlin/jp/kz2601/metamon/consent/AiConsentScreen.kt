package jp.kz2601.metamon.consent

import android.content.Intent
import android.net.Uri
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import jp.kz2601.metamon.R

/**
 * 第三者 AI(Anthropic の Claude)へ送る前の説明と同意(iOS の `AIConsentView`・#888)。
 * 初回起動時・同意前に AI を使おうとした時・設定から出す。「あとで」でも閲覧・記録などは使える。
 * 文言は iOS と同じ(strings_consent.xml)。
 */
@Composable
fun AiConsentDialog(needsReconsent: Boolean, onAgree: () -> Unit, onLater: () -> Unit) {
    Dialog(
        onDismissRequest = onLater,
        // 外側のタップでは閉じない(iOS の interactiveDismissDisabled)。戻るは「あとで」
        properties = DialogProperties(usePlatformDefaultWidth = false, dismissOnClickOutside = false),
    ) {
        Surface(Modifier.fillMaxSize().testTag("consent.sheet")) {
            Column(Modifier.fillMaxSize()) {
                Column(
                    Modifier.weight(1f).verticalScroll(rememberScrollState()).padding(24.dp),
                    verticalArrangement = Arrangement.spacedBy(16.dp),
                ) {
                    Text(
                        stringResource(R.string.consent_title),
                        style = MaterialTheme.typography.titleLarge.copy(fontWeight = FontWeight.Bold),
                        textAlign = TextAlign.Center,
                        modifier = Modifier.fillMaxWidth().padding(top = 16.dp),
                    )
                    if (needsReconsent) {
                        Text(
                            stringResource(R.string.consent_reconsent),
                            style = MaterialTheme.typography.bodyMedium.copy(fontWeight = FontWeight.Bold),
                            modifier = Modifier.testTag("consent.reconsent"),
                        )
                    }
                    Text(stringResource(R.string.consent_intro), style = MaterialTheme.typography.bodyMedium)
                    Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                        Bullet(stringResource(R.string.consent_item_utterance))
                        Bullet(stringResource(R.string.consent_item_chat))
                        Bullet(stringResource(R.string.consent_item_design))
                    }
                    Note(stringResource(R.string.consent_destination))
                    Note(stringResource(R.string.consent_no_records))
                    Note(stringResource(R.string.consent_ai_button))
                    Note(stringResource(R.string.consent_ai_chat))
                    Note(stringResource(R.string.consent_without))
                    LegalLinks()
                }
                Column(
                    Modifier.fillMaxWidth().padding(24.dp),
                    horizontalAlignment = Alignment.CenterHorizontally,
                    verticalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    Button(onClick = onAgree, modifier = Modifier.fillMaxWidth().testTag("consent.agree")) {
                        Text(stringResource(R.string.consent_agree))
                    }
                    TextButton(onClick = onLater, modifier = Modifier.testTag("consent.later")) {
                        Text(stringResource(R.string.consent_later))
                    }
                }
            }
        }
    }
}

/** プライバシーポリシー・利用規約(公開の URL をブラウザで開く。iOS の `LegalDocument.publicURL`)。 */
@Composable
fun LegalLinks(modifier: Modifier = Modifier) {
    val context = LocalContext.current
    Column(modifier) {
        LegalDocument.entries.forEach { doc ->
            TextButton(onClick = {
                runCatching { context.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(doc.url))) }
            }) { Text(stringResource(doc.title)) }
        }
    }
}

enum class LegalDocument(val url: String, val title: Int) {
    PRIVACY_POLICY("https://tailor-it.app/privacy", R.string.legal_privacy_policy),
    TERMS("https://tailor-it.app/terms", R.string.legal_terms),
}

@Composable
private fun Bullet(text: String) {
    Row {
        Text("・", style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.primary)
        Text(text, style = MaterialTheme.typography.bodyMedium)
    }
}

@Composable
private fun Note(text: String) {
    Text(text, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
}
