package jp.kz2601.metamon.desktop.onboarding

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.widthIn
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.paneTitle
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.desktop.HostOs
import jp.kz2601.metamon.desktop.Keymap
import jp.kz2601.metamon.desktop.Shortcut
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.services.consent.KeyValueStore

/**
 * 初回の流れ(#3201・iOS の `FirstRunTutorial` のデスクトップ版)。初めての起動で 3 枚のダイアログを順に出す。
 * Esc・「あとで見る」は閉じる(次の起動で自動では出さない)。F1 の一覧・コマンドパレットの「使い方を見る」から見直せる。
 */
object OnboardingCards {
    /** 1 枚。[body] は OS のキーの名前を入れて作る。 */
    class Card(val id: String, val titleKey: String, val body: (HostOs) -> String, val noteKey: String? = null) {
        val title: String get() = Strings[titleKey]
    }

    /** 出す順(作る → キーボード → ウィンドウ)。1 枚目の文言は Android・iOS と同じキー。 */
    val all: List<Card> = listOf(
        Card("create", "onboarding_create_title", { Strings["onboarding_create_body"] }, noteKey = "onboarding_create_note"),
        Card("keys", "desktop_onboarding_keys_title", { os ->
            Strings.format("desktop_onboarding_keys_body", Keymap.label(Shortcut.PALETTE, os), Keymap.label(Shortcut.HELP, os))
        }),
        Card("windows", "desktop_onboarding_windows_title", { Strings["desktop_onboarding_windows_body"] }),
    )

    /** 読み上げ・見た目の「1 / 3」。 */
    fun pageLabel(index: Int, count: Int = all.size): String = Strings.format("onboarding_page", index + 1, count)
}

/** 何枚目か(画面なしで確かめる)。 */
class OnboardingPager(val count: Int = OnboardingCards.all.size) {
    var index: Int = 0
        private set

    val isFirst: Boolean get() = index == 0
    val isLast: Boolean get() = index == count - 1

    /** 次へ。最後の 1 枚なら false(終わり)。 */
    fun next(): Boolean {
        if (isLast) return false
        index++
        return true
    }

    fun back() {
        if (index > 0) index--
    }
}

/**
 * 見たかを端末の設定(`settings.json`)に残す。鍵は iOS と同じ `metamon.tutorial.seen`。
 * サインアウトの後片付け(settings を消す)で未読に戻る(次の人に出す)。
 */
class OnboardingStore(private val store: KeyValueStore) {
    val hasSeen: Boolean get() = store.getBoolean(SEEN_KEY) == true

    /** 起動時に自動で出すか。 */
    fun shouldAutoPresent(): Boolean = !hasSeen

    /** 最後まで見た・「あとで見る」・Esc。次の起動では出さない。 */
    fun markSeen() = store.putBoolean(SEEN_KEY, true)

    fun reset() = store.remove(SEEN_KEY)

    /** `TAILORIT_RESET_ONBOARDING=1` で見ていないことにする(開発・確かめ用)。戻したか。 */
    fun applyLaunchOverride(value: String? = System.getenv(ENV_RESET)): Boolean {
        if (value?.trim()?.lowercase() !in RESET_VALUES) return false
        reset()
        return true
    }

    companion object {
        const val SEEN_KEY = "metamon.tutorial.seen"
        const val ENV_RESET = "TAILORIT_RESET_ONBOARDING"
        private val RESET_VALUES = setOf("1", "true", "yes")
    }
}

/**
 * 初回の流れのダイアログ。[onFinish] は最後まで見た時(`completed = true`)と「あとで見る」・Esc(false)。
 * 読み上げはダイアログの名前(「使い方」)と今の枚数(「1 / 3」)を知らせる。
 */
@Composable
fun OnboardingDialog(os: HostOs = HostOs.current, onFinish: (completed: Boolean) -> Unit) {
    val pager = remember { OnboardingPager() }
    var page by remember { mutableIntStateOf(0) }
    val card = OnboardingCards.all[page]
    val name = Strings["desktop_onboarding_title"]
    val pageLabel = OnboardingCards.pageLabel(page)
    AlertDialog(
        onDismissRequest = { onFinish(false) },
        modifier = Modifier.widthIn(max = 520.dp).semantics { paneTitle = name },
        title = { Text(card.title, modifier = Modifier.semantics { heading() }) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(card.body(os))
                card.noteKey?.let { Text(Strings[it], style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant) }
                Text(
                    pageLabel,
                    style = MaterialTheme.typography.labelMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.semantics { liveRegion = LiveRegionMode.Polite },
                )
            }
        },
        confirmButton = {
            Button(onClick = {
                if (pager.next()) page = pager.index else onFinish(true)
            }) { Text(Strings[if (pager.isLast) "onboarding_start" else "onboarding_next"]) }
        },
        dismissButton = {
            Row(horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                TextButton(onClick = { onFinish(false) }) { Text(Strings["desktop_onboarding_later"]) }
                if (!pager.isFirst) TextButton(onClick = { pager.back(); page = pager.index }) { Text(Strings["renderer_back"]) }
            }
        },
    )
}
