package jp.kz2601.metamon.renderer.components.actions

import androidx.annotation.StringRes
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentStyle
import jp.kz2601.metamon.dsl.logic.ExternalUrlPolicy
import jp.kz2601.metamon.dsl.logic.OpenApp
import jp.kz2601.metamon.renderer.R
import jp.kz2601.metamon.renderer.form.FormDraft
import jp.kz2601.metamon.renderer.registry.RenderContext

/**
 * `openUrlButton.app` / `to`(iOS #989)。知らない app(新しい版)は UNKNOWN の空文字になり、[OpenUrlTarget.resolve] で開かない。
 */
internal val Component.openAppName: String? get() = app?.wire
internal val Component.openAppTo: String? get() = to

/** openUrlButton が開く先(iOS の `OpenUrlButtonLeaf.body` の分岐)。 */
sealed interface OpenUrlTarget {
    /** app で開く。[urls] は先頭から試す候補(空 = phone で番号がまだ無い・押せない)。 */
    data class App(val app: OpenApp, val urls: List<String>) : OpenUrlTarget

    data class Url(val url: String) : OpenUrlTarget

    /** 許可していない URL・知らない app。 */
    data object Invalid : OpenUrlTarget

    companion object {
        /** app が分かればそれを優先。app を書いて分からない名前なら url があっても開かない(iOS と同じ)。 */
        fun resolve(app: String?, to: String?, url: String?, form: FormDraft?): OpenUrlTarget {
            OpenApp.of(app)?.let { a -> return App(a, a.urls(to?.let { FormTemplate.fill(it, form) })) }
            if (app != null) return Invalid
            return ExternalUrlPolicy.openable(url)?.let(::Url) ?: Invalid
        }
    }
}

@StringRes
internal fun OpenApp.labelRes(): Int = when (this) {
    OpenApp.PAYPAY -> R.string.renderer_open_paypay
    OpenApp.LINE -> R.string.renderer_open_line
    OpenApp.SLACK -> R.string.renderer_open_slack
    OpenApp.DISCORD -> R.string.renderer_open_discord
    OpenApp.X -> R.string.renderer_open_x
    OpenApp.INSTAGRAM -> R.string.renderer_open_instagram
    OpenApp.MAPS -> R.string.renderer_open_maps
    OpenApp.MAIL -> R.string.renderer_open_mail
    OpenApp.PHONE -> R.string.renderer_open_phone
}

/**
 * `openUrlButton`: 固定 URL か app(+ to)を開く。style は primary | secondary | plain(既定 secondary)。
 * url は https と allowlist のスキームだけ(https は Custom Tabs)。app は対応アプリ → 無ければ Web の順で開く。
 */
@Composable
fun OpenUrlButtonLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) {
    val context = LocalContext.current
    val kind = ActionButtonKind.of(c.style.takeIf { it != ComponentStyle.DESTRUCTIVE }, default = ActionButtonKind.SECONDARY)
    val fill = c.style != ComponentStyle.PLAIN
    when (val target = OpenUrlTarget.resolve(c.openAppName, c.openAppTo, c.url, ctx.form)) {
        is OpenUrlTarget.App -> ActionButton(
            label = c.label ?: stringResource(target.app.labelRes()),
            kind = kind,
            onClick = { openExternal(context, target.urls, preferApp = true) },
            modifier = modifier,
            icon = c.icon ?: target.app.defaultIcon,
            enabled = target.urls.isNotEmpty(),
            fillWidth = fill,
        )
        is OpenUrlTarget.Url -> ActionButton(
            label = c.label ?: target.url,
            kind = kind,
            onClick = { openExternal(context, listOf(target.url)) },
            modifier = modifier,
            icon = c.icon ?: "safari",
            fillWidth = fill,
        )
        OpenUrlTarget.Invalid -> ActionNotice(stringResource(R.string.renderer_open_url_invalid), modifier)
    }
}
