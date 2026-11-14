package jp.kz2601.metamon.desktop

import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.material3.Button
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentStyle
import jp.kz2601.metamon.dsl.logic.ExternalUrlPolicy
import jp.kz2601.metamon.dsl.logic.OpenApp
import jp.kz2601.metamon.renderer.components.actions.FormTemplate
import jp.kz2601.metamon.renderer.form.FormDraft

/** openUrlButton が開く先(Android の `OpenUrlTarget`・iOS の `OpenUrlButtonLeaf.body` の分岐)。 */
sealed interface OpenUrlTarget {
    /** app で開く。[urls] は先頭から試す候補(空 = phone で番号がまだ無い・押せない)。 */
    data class App(val app: OpenApp, val urls: List<String>) : OpenUrlTarget

    data class Url(val url: String) : OpenUrlTarget

    /** 許可していない URL・知らない app。 */
    data object Invalid : OpenUrlTarget

    companion object {
        /** app が分かればそれを優先。app を書いて分からない名前なら url があっても開かない。`to` の `{{fieldId}}` はフォームの値で埋める。 */
        fun resolve(c: Component, form: FormDraft?): OpenUrlTarget {
            val app = c.app?.wire
            OpenApp.of(app)?.let { a -> return App(a, a.urls(c.to?.let { FormTemplate.fill(it, form) })) }
            if (app != null) return Invalid
            return ExternalUrlPolicy.openable(c.url)?.let(::Url) ?: Invalid
        }
    }
}

/** app の既定ラベル(Android と同じ文言のキー)。 */
internal fun OpenApp.labelKey(): String = "renderer_open_$wire"

/**
 * `openUrlButton`: 固定 URL か app(+ to)を開く。開くのは link と同じ openUrl(許可した scheme だけ・既定のブラウザ / アプリ)。
 * app の候補は先頭から試し、開けなければ次(slack:// が無いパソコンは Web)。style は primary | secondary | plain(既定 secondary)。
 */
@Composable
fun OpenUrlButtonLeaf(c: Component, form: FormDraft?, open: (List<String>) -> Unit) {
    val target = OpenUrlTarget.resolve(c, form)
    val (label, urls, defaultIcon) = when (target) {
        is OpenUrlTarget.App -> Triple(c.label ?: Strings[target.app.labelKey()], target.urls, target.app.defaultIcon)
        is OpenUrlTarget.Url -> Triple(c.label ?: target.url, listOf(target.url), "safari")
        OpenUrlTarget.Invalid -> {
            Text(Strings["renderer_open_url_invalid"], color = Color.Gray, style = MaterialTheme.typography.bodySmall)
            return
        }
    }
    val enabled = urls.isNotEmpty()
    val onClick = { open(urls) }
    val content: @Composable () -> Unit = {
        Row(verticalAlignment = Alignment.CenterVertically) {
            DesktopIcons.vector(c.icon ?: defaultIcon)?.let {
                Icon(it, contentDescription = null, modifier = Modifier.size(18.dp))
                Spacer(Modifier.width(8.dp))
            }
            Text(label)
        }
    }
    when (c.style) {
        ComponentStyle.PRIMARY -> Button(onClick = onClick, enabled = enabled) { content() }
        ComponentStyle.PLAIN -> TextButton(onClick = onClick, enabled = enabled) { content() }
        else -> OutlinedButton(onClick = onClick, enabled = enabled) { content() }
    }
}
