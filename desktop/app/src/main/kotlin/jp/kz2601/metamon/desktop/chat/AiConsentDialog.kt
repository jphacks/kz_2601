package jp.kz2601.metamon.desktop.chat

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.services.consent.AiConsentStore

/**
 * AI へ送ることの説明と同意(#2160)。会話を開く前に、同意がまだ無い時だけ出す。
 * 文言は Android の consent_* をそのまま使う(版と鍵は AiConsentStore・iOS / Android と同じ)。
 */
object AiConsentGate {
    /** 会話を開く前に同意の説明を出すか。 */
    fun needsPrompt(store: AiConsentStore?): Boolean = store != null && !store.isGranted

    /** 説明の段落(見出しの下に並べる順)。版が上がった人には最初に出し直しの一文。 */
    fun paragraphs(reconsent: Boolean): List<String> = buildList {
        if (reconsent) add(Strings["consent_reconsent"])
        add(Strings["consent_intro"])
        listOf("consent_item_utterance", "consent_item_design", "consent_item_chat").forEach { add("・" + Strings[it]) }
        add(Strings["consent_no_records"])
        add(Strings["consent_destination"])
        add(Strings["consent_without"])
    }
}

@Composable
fun AiConsentDialog(store: AiConsentStore, onAgree: () -> Unit, onLater: () -> Unit) {
    AlertDialog(
        onDismissRequest = onLater,
        title = { Text(Strings["consent_title"]) },
        text = {
            Column(Modifier.heightIn(max = 420.dp).verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                AiConsentGate.paragraphs(store.state.value.needsReconsent).forEach { Text(it, style = MaterialTheme.typography.bodyMedium) }
            }
        },
        confirmButton = { TextButton(onClick = onAgree) { Text(Strings["consent_agree"]) } },
        dismissButton = { TextButton(onClick = onLater) { Text(Strings["consent_later"]) } },
    )
}
