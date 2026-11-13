package jp.kz2601.metamon.desktop.voice

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.selected
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.desktop.i18n.Strings

/** 頼み方のコツのダイアログの文言(Android の voice_tips_title・desktop_tips_*)。 */
object PromptTipsText {
    val TITLE get() = Strings["voice_tips_title"]
    val INTRO get() = Strings["desktop_tips_intro"]
    val COPY get() = Strings["desktop_tips_copy"]
    val COPIED get() = Strings["desktop_tips_copied"]
    val COPY_FAILED get() = Strings["desktop_tips_copy_failed"]
    val CLOSE get() = Strings["desktop_close"]
}

/**
 * 頼み方のコツ: カテゴリごとの見出し(読み上げの見出し)と例の一覧。例を選ぶと「この文をコピー」が出る。
 * [onCopy] は文をクリップボードへ入れて、入れられたかを返す。会話の画面が入ったら入力欄に入れる。Esc で閉じる。
 */
@Composable
fun PromptTipsDialog(
    onCopy: (String) -> Boolean,
    onMessage: (String) -> Unit,
    onDismiss: () -> Unit,
) {
    val categories = remember { PromptTips.categories() }
    var selected by remember { mutableStateOf<String?>(null) }
    AlertDialog(
        onDismissRequest = onDismiss,
        modifier = Modifier.widthIn(max = 640.dp),
        title = { Text(PromptTipsText.TITLE, modifier = Modifier.semantics { heading() }) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(PromptTipsText.INTRO, style = MaterialTheme.typography.bodySmall, color = Color.Gray)
                LazyColumn(Modifier.fillMaxWidth().heightIn(max = 420.dp), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                    categories.forEach { c ->
                        item(key = "cat:${c.id}") {
                            Text(
                                Strings[c.titleKey],
                                style = MaterialTheme.typography.titleSmall,
                                modifier = Modifier.padding(top = 12.dp, bottom = 4.dp).semantics { heading() },
                            )
                        }
                        c.tips.forEach { t ->
                            item(key = "tip:${t.id}") {
                                val on = selected == t.id
                                val text = Strings[t.textKey]
                                Column(
                                    Modifier.fillMaxWidth()
                                        .background(if (on) MaterialTheme.colorScheme.secondaryContainer else Color.Transparent, MaterialTheme.shapes.small)
                                        .clickable { selected = t.id }
                                        .semantics { this.selected = on }
                                        .padding(horizontal = 10.dp, vertical = 8.dp),
                                ) {
                                    Text(text)
                                    if (on) {
                                        TextButton(onClick = { onMessage(if (onCopy(text)) PromptTipsText.COPIED else PromptTipsText.COPY_FAILED) }) {
                                            Text(PromptTipsText.COPY)
                                        }
                                    }
                                }
                            }
                        }
                    }
                }
            }
        },
        confirmButton = { TextButton(onClick = onDismiss) { Text(PromptTipsText.CLOSE) } },
    )
}
