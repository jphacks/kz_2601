package jp.kz2601.metamon.renderer.components.actions

import androidx.annotation.StringRes
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.logic.ShareTarget
import jp.kz2601.metamon.renderer.R
import jp.kz2601.metamon.renderer.components.PhotoValue
import jp.kz2601.metamon.renderer.form.FormDraft
import jp.kz2601.metamon.renderer.registry.RenderContext

/** 送り先(any・line・slack・discord・x・mail)。知らない値は共有シートに落ちる。 */
internal val Component.shareTarget: ShareTarget get() = ShareTarget.of(shareTo?.wire)

/** 押した時にすること(iOS の `ShareButtonLeaf.body` の分岐)。 */
sealed interface ShareAction {
    /** LINE・X・メール: 文面を載せた URL を直接開く(写真は渡せない)。開けなければ [text] を共有シートで。 */
    data class Direct(val url: String, val text: String) : ShareAction

    /** any・slack・discord: OS の共有シート。 */
    data class Sheet(val text: String) : ShareAction

    /** any・slack・discord で `field` が写真(data URL)の時: 写真と文面を共有シートで(#1086)。 */
    data class Photo(val text: String, val dataUrl: String) : ShareAction

    companion object {
        fun of(target: ShareTarget, text: String?, field: String?, form: FormDraft?): ShareAction {
            val body = target.format(FormTemplate.shareText(text, field, form))
            target.directUrl(body)?.let { return Direct(it, body) }
            return ShareImages.photoDataUrl(field, form)?.let { Photo(body, it) } ?: Sheet(body)
        }
    }
}

@StringRes
internal fun ShareTarget.labelRes(): Int = when (this) {
    ShareTarget.ANY -> R.string.renderer_share_any
    ShareTarget.LINE -> R.string.renderer_share_line
    ShareTarget.SLACK -> R.string.renderer_share_slack
    ShareTarget.DISCORD -> R.string.renderer_share_discord
    ShareTarget.X -> R.string.renderer_share_x
    ShareTarget.MAIL -> R.string.renderer_share_mail
}

/**
 * `shareButton`: text の `{{fieldId}}` を下書きの値で埋めて送る。shareTo: any = 共有シート / line・x・mail = 直接の URL /
 * slack・discord = 共有シート(文面を Markdown 記号なしに整える)。見た目は secondary(iOS と同じ)。
 * field が photo(data URL)なら写真も共有シートに載せる(#1086・LINE・X・メールは URL なので文面だけ)。
 */
@Composable
fun ShareButtonLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) {
    val context = LocalContext.current
    val target = c.shareTarget
    val label = c.label ?: stringResource(target.labelRes())
    ActionButton(
        label = label,
        kind = ActionButtonKind.SECONDARY,
        onClick = {
            when (val a = ShareAction.of(target, c.text, c.field, ctx.form)) {
                is ShareAction.Direct -> if (!openExternal(context, listOf(a.url), preferApp = true)) shareText(context, a.text, label)
                is ShareAction.Sheet -> shareText(context, a.text, label)
                is ShareAction.Photo -> PhotoValue.dataBytes(a.dataUrl)
                    ?.let { shareImage(context, it, ShareImages.PHOTO_FILE, ShareImages.JPEG, a.text, label) }
                    ?: shareText(context, a.text, label)
            }
        },
        modifier = modifier,
        icon = c.icon ?: target.defaultIcon,
    )
}
