package jp.kz2601.metamon.desktop.stage

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedCard
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.services.consent.KeyValueStore

/**
 * 最初のアプリの完成の後に 1 回だけ出す「こう頼むと直せます」(#3360・Android の `EditHintStore`・iOS の `FirstRunTutorialStore` の editHint)。
 * 印は設定の JSON(`DesktopStore.settings`)に持つので、「この端末のデータを消す」・サインアウトで消え、次の人には出し直す(Android の `forget` と同じ)。
 * 完成の後のお知らせの順番(要約 → 保存期間 → これ)は呼び出し側が持つ。ここは出すかと見た印だけ。
 */
class EditHintStore(private val store: KeyValueStore) {
    val hasShown: Boolean get() = store.getBoolean(KEY) == true

    /** 初回の流れから作った最初のアプリの直後で、まだ出していなければ出す */
    fun shouldShow(createdFromInitial: Boolean): Boolean = createdFromInitial && !hasShown

    fun markShown() = store.putBoolean(KEY, true)

    /** 出すなら印を付けて true(出した時点で覚える。閉じ方を問わず 2 回目は出さない) */
    fun take(createdFromInitial: Boolean): Boolean {
        if (!shouldShow(createdFromInitial)) return false
        markShown()
        return true
    }

    companion object {
        const val KEY = "tailorit.editHintShown"
    }
}

/**
 * 「こう頼むと直せます」のカード(Mac の `MacEditHintBar` と同じ言い方)。Android の「マイクをタップ」は文字でも頼めるデスクトップに合わせて言い換え、
 * 例文と「とじる」は Android の `feedback_edit_hint_*` をそのまま使う。[onTalk] は入力欄へ移すなど(呼び出し側)。
 */
@Composable
fun FirstEditHintCard(onClose: () -> Unit, onTalk: () -> Unit, modifier: Modifier = Modifier) {
    OutlinedCard(modifier.widthIn(max = 480.dp).fillMaxWidth().semantics { liveRegion = LiveRegionMode.Polite }) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                Text(
                    Strings["desktop_edit_hint_title"],
                    style = MaterialTheme.typography.titleMedium,
                    fontWeight = FontWeight.SemiBold,
                    modifier = Modifier.semantics { heading() },
                )
                Text(Strings["desktop_edit_hint_body"], style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            Text(Strings["feedback_edit_hint_examples"], style = MaterialTheme.typography.bodyMedium, fontWeight = FontWeight.Medium)
            Row(verticalAlignment = Alignment.CenterVertically) {
                TextButton(onClick = onClose) { Text(Strings["feedback_edit_hint_close"]) }
                Spacer(Modifier.weight(1f))
                Button(onClick = onTalk) { Text(Strings["desktop_edit_hint_try"]) }
            }
        }
    }
}
