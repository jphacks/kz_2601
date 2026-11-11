package jp.kz2601.metamon.renderer.components

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.Size
import jp.kz2601.metamon.dsl.records.isFollow
import jp.kz2601.metamon.renderer.R
import jp.kz2601.metamon.renderer.components.actions.ActionButton
import jp.kz2601.metamon.renderer.components.actions.ActionButtonKind
import jp.kz2601.metamon.renderer.components.social.follows
import jp.kz2601.metamon.renderer.components.social.showsFollowButton
import jp.kz2601.metamon.renderer.form.FormDraft
import jp.kz2601.metamon.renderer.registry.RenderContext
import jp.kz2601.metamon.renderer.theme.RenderTheme

// #357 で入れた最小の Leaf(button・voiceInput)。表示系の Leaf は 1 部品 1 ファイル(TextLeaf.kt など・#857)。

/** button が見えるか(iOS の `ButtonLeaf.isVisible`)。 */
fun isButtonVisible(action: Action?, isEditing: Boolean = false, readOnly: Boolean = false, canDelete: Boolean = true): Boolean =
    when (action?.type) {
        ActionType.DELETE_RECORD -> isEditing && canDelete
        ActionType.SAVE_RECORD -> !readOnly
        else -> true
    }

/** フォームの下書きから見えるかを決める(フォームの外は新規のフォーム扱い)。 */
fun isButtonVisible(action: Action?, form: FormDraft?): Boolean =
    isButtonVisible(action, form?.isEditing ?: false, form?.isReadOnly ?: false, form?.canDelete ?: true)

/** icon 省略時の既定(iOS の `ButtonLeaf.defaultIcon`)。押すと共有・カレンダー・本の検索になるものだけ。 */
fun buttonDefaultIcon(action: Action?): String? = when (action?.type) {
    ActionType.EXPORT -> "square.and.arrow.up"
    ActionType.ADD_TO_CALENDAR -> "calendar.badge.plus"
    ActionType.LOOKUP_BOOK -> "text.book.closed"
    else -> null
}

/**
 * `button`: style(primary | secondary | destructive | plain・既定 primary)。押すと action を dispatch する
 * (confirmMessage の確認は runtime が出す)。見た目は iOS の `RenderButtonStyle`([ActionButton])。
 */
@Composable
fun ButtonLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) {
    if (!isButtonVisible(c.action, ctx.form)) return
    // follow / unfollow(#270)は今の投稿の作者の状態に合う方だけ
    if (c.action?.type?.isFollow == true) ctx.runtime.follows?.followsVersion?.collectAsState()?.value
    if (!ctx.runtime.showsFollowButton(c.action, ctx.form?.recordId)) return
    ActionButton(
        label = c.label.orEmpty(),
        kind = ActionButtonKind.of(c.style),
        onClick = { c.action?.let(ctx.perform) },
        modifier = modifier,
        icon = c.icon ?: buttonDefaultIcon(c.action),
    )
}

/** 音声の入口(仮)。認識は #359。 */
@Composable
fun VoiceInputLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) {
    Column(
        modifier = modifier.fillMaxWidth().padding(vertical = 48.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.lg),
    ) {
        Text(
            c.prompt ?: stringResource(R.string.inputs_voice_prompt),
            style = MaterialTheme.typography.headlineSmall,
            textAlign = TextAlign.Center,
        )
        val buttonSize = if (c.size == Size.LARGE) 120.dp else 72.dp
        Button(
            onClick = { ctx.runtime.onVoiceInput(c.id) },
            shape = CircleShape,
            modifier = Modifier.size(buttonSize),
        ) {
            Text(stringResource(R.string.inputs_voice_talk), style = MaterialTheme.typography.titleMedium)
        }
    }
}
