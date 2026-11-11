package jp.kz2601.metamon.renderer.components.social

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import jp.kz2601.metamon.renderer.R
import jp.kz2601.metamon.renderer.theme.RenderTheme
import kotlinx.coroutines.flow.StateFlow

/** 通報の理由(`POST /v1/reports` の reason)。並びと wire は :services の ReportReason と同じ(iOS の選択肢の順)。 */
enum class ReportReasonOption(val wire: String) {
    SPAM("spam"),
    SEXUAL("sexual"),
    VIOLENCE("violence"),
    HARASSMENT("harassment"),
    ILLEGAL("illegal"),
    PRIVACY("privacy"),
    OTHER("other"),
}

/** 通報・ブロック・仲間探しの結果の一言(iOS の `ModerationNotice`)。 */
enum class SocialNotice { REPORTED, REPORT_LIMITED, REPORT_FAILED, BLOCKED, BLOCK_FAILED }

/** ホストが出す確認 1 つ(iOS の ModerationDialogs・MatchingSheets)。[ModerationHost] が出し、[SocialPromptHost] で答える。 */
sealed interface SocialPrompt {
    /** 通報の理由を選ぶ。 */
    data class Report(val authorName: String?) : SocialPrompt

    /** 作者のブロックの確認。 */
    data class Block(val authorName: String?) : SocialPrompt

    /** 結果の一言。[name] は BLOCKED の相手の名前。 */
    data class Notice(val kind: SocialNotice, val name: String? = null) : SocialPrompt

    /** 「18 歳以上ですか?」。初回の「つながりたい」・承認の前に 1 回だけ。 */
    data object AdultConfirm : SocialPrompt
}

/** [SocialPrompt] を出して答えを返すホスト(:app の AppRuntime)。答えるとホストが次の prompt(結果の一言)に進めるか閉じる。 */
interface SocialPromptHost {
    val socialPrompt: StateFlow<SocialPrompt?>

    fun submitReport(reason: ReportReasonOption)

    fun confirmBlock()

    /** 「18 歳以上です」: 端末に覚えて、止めていた操作を続ける(年齢そのものは保存しない)。 */
    fun confirmAdult()

    /** 「18 歳未満です」: 何も覚えず、送らない。 */
    fun declineAdult()

    fun dismissSocialPrompt()
}

/** DSLView が 1 つだけ置く。ダイアログは窓なのでシート・全画面の上にも出る。 */
@Composable
fun SocialDialogs(host: SocialPromptHost) {
    val prompt by host.socialPrompt.collectAsState()
    when (val p = prompt) {
        null -> Unit
        is SocialPrompt.Report -> ReportReasonDialog(host)
        is SocialPrompt.Block -> AlertDialog(
            onDismissRequest = host::dismissSocialPrompt,
            title = { Text(stringResource(R.string.social_block_title, p.authorName ?: stringResource(R.string.social_anonymous))) },
            text = { Text(stringResource(R.string.social_block_message)) },
            confirmButton = {
                TextButton(onClick = host::confirmBlock, modifier = Modifier.testTag("moderation.block.confirm")) {
                    Text(stringResource(R.string.social_block_confirm), color = MaterialTheme.colorScheme.error)
                }
            },
            dismissButton = { TextButton(onClick = host::dismissSocialPrompt) { Text(stringResource(R.string.social_cancel)) } },
        )
        is SocialPrompt.Notice -> AlertDialog(
            onDismissRequest = host::dismissSocialPrompt,
            title = { Text(noticeTitle(p.kind)) },
            text = { Text(noticeMessage(p)) },
            confirmButton = { TextButton(onClick = host::dismissSocialPrompt) { Text(stringResource(R.string.social_ok)) } },
        )
        SocialPrompt.AdultConfirm -> AdultConfirmDialog(host)
    }
}

@Composable
private fun ReportReasonDialog(host: SocialPromptHost) {
    AlertDialog(
        onDismissRequest = host::dismissSocialPrompt,
        title = { Text(stringResource(R.string.social_report_title)) },
        text = {
            Column(Modifier.verticalScroll(rememberScrollState())) {
                Text(stringResource(R.string.social_report_message), style = MaterialTheme.typography.bodyMedium)
                ReportReasonOption.entries.forEach { reason ->
                    TextButton(
                        onClick = { host.submitReport(reason) },
                        modifier = Modifier.fillMaxWidth().testTag("moderation.report.${reason.wire}"),
                    ) { Text(reasonLabel(reason), Modifier.fillMaxWidth()) }
                }
            }
        },
        confirmButton = {},
        dismissButton = { TextButton(onClick = host::dismissSocialPrompt) { Text(stringResource(R.string.social_cancel)) } },
    )
}

@Composable
private fun AdultConfirmDialog(host: SocialPromptHost) {
    AlertDialog(
        // 外を押して閉じたら「何もしない」(送らない・覚えない)
        onDismissRequest = host::dismissSocialPrompt,
        title = { Text(stringResource(R.string.social_adult_title), textAlign = TextAlign.Center, modifier = Modifier.fillMaxWidth()) },
        text = {
            Column(Modifier.verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.md)) {
                Text(stringResource(R.string.social_adult_message), style = MaterialTheme.typography.bodyMedium)
                Text(
                    stringResource(R.string.social_adult_footnote),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                Button(onClick = host::confirmAdult, modifier = Modifier.fillMaxWidth().testTag("matching.adult.confirm")) {
                    Text(stringResource(R.string.social_adult_confirm))
                }
                OutlinedButton(onClick = host::declineAdult, modifier = Modifier.fillMaxWidth().testTag("matching.adult.decline")) {
                    Text(stringResource(R.string.social_adult_decline))
                }
            }
        },
        confirmButton = {},
    )
}

@Composable
private fun reasonLabel(reason: ReportReasonOption): String = stringResource(
    when (reason) {
        ReportReasonOption.SPAM -> R.string.social_reason_spam
        ReportReasonOption.SEXUAL -> R.string.social_reason_sexual
        ReportReasonOption.VIOLENCE -> R.string.social_reason_violence
        ReportReasonOption.HARASSMENT -> R.string.social_reason_harassment
        ReportReasonOption.ILLEGAL -> R.string.social_reason_illegal
        ReportReasonOption.PRIVACY -> R.string.social_reason_privacy
        ReportReasonOption.OTHER -> R.string.social_reason_other
    },
)

@Composable
private fun noticeTitle(kind: SocialNotice): String = stringResource(
    when (kind) {
        SocialNotice.REPORTED -> R.string.social_reported_title
        SocialNotice.REPORT_LIMITED, SocialNotice.REPORT_FAILED -> R.string.social_report_failed_title
        SocialNotice.BLOCKED -> R.string.social_blocked_title
        SocialNotice.BLOCK_FAILED -> R.string.social_block_failed_title
    },
)

@Composable
private fun noticeMessage(n: SocialPrompt.Notice): String = when (n.kind) {
    SocialNotice.REPORTED -> stringResource(R.string.social_reported_message)
    SocialNotice.REPORT_LIMITED -> stringResource(R.string.social_report_limited)
    SocialNotice.REPORT_FAILED, SocialNotice.BLOCK_FAILED -> stringResource(R.string.social_try_again)
    SocialNotice.BLOCKED -> stringResource(R.string.social_blocked_message, n.name ?: stringResource(R.string.social_anonymous))
}
