package jp.kz2601.metamon.desktop.privacy

import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.desktop.io.ExportClipboard
import jp.kz2601.metamon.services.privacy.PublicDataPrompt
import kotlinx.serialization.json.JsonObject

/** 生成の直後の「個人の情報も公開されます」の確認(#253・#1738)。 */
enum class PublicDataChoice { KEEP, REMOVE_FIELDS, MAKE_PRIVATE }

object PublicData {
    /** ai が `meta.publicPersonalData` を返した時だけ確認を出す(無い・空・壊れた欄だけなら null)。 */
    fun prompt(meta: List<JsonObject>, version: Int): PublicDataPrompt? = PublicDataPrompt.fromMeta(meta, version)

    /** 選んだ内容 → 会話に送る直しの指示文(公開のままは送らないので null)。 */
    fun instruction(prompt: PublicDataPrompt, choice: PublicDataChoice, lang: Lang = Strings.lang): String? {
        val english = lang == Lang.EN
        return when (choice) {
            PublicDataChoice.KEEP -> null
            PublicDataChoice.REMOVE_FIELDS -> prompt.removeInstruction(english)
            PublicDataChoice.MAKE_PRIVATE -> prompt.privateInstruction(english)
        }
    }

    /** 会話の画面が入るまでの送り先: 指示文をクリップボードへ。出す一言を返す。 */
    fun copyInstruction(text: String): String =
        if (ExportClipboard.put(text)) Strings["desktop_public_data_copied"] else Strings["desktop_io_copy_failed"]
}

/**
 * 「公開のままにする / この欄を消す / 自分だけにする」。後の 2 つは [onSend] に直しの指示文を渡す
 * (会話の画面が入るまでの [onSend] は [PublicData.copyInstruction])。どれを選んでも [onDone] で閉じる。
 */
@Composable
fun PublicDataDialog(prompt: PublicDataPrompt, onSend: (String) -> Unit, onDone: () -> Unit) {
    fun choose(choice: PublicDataChoice) {
        PublicData.instruction(prompt, choice)?.let(onSend)
        onDone()
    }
    AlertDialog(
        onDismissRequest = { choose(PublicDataChoice.KEEP) },
        title = { Text(Strings["desktop_public_data_title"]) },
        text = { Text(Strings.format("desktop_public_data_body", prompt.fieldNames(Strings.lang == Lang.EN))) },
        confirmButton = { TextButton(onClick = { choose(PublicDataChoice.REMOVE_FIELDS) }) { Text(Strings["desktop_public_data_remove"]) } },
        dismissButton = {
            TextButton(onClick = { choose(PublicDataChoice.MAKE_PRIVATE) }) { Text(Strings["desktop_public_data_private"]) }
            TextButton(onClick = { choose(PublicDataChoice.KEEP) }) { Text(Strings["desktop_public_data_keep"]) }
        },
    )
}
