package jp.kz2601.metamon.account

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.AccountCircle
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import jp.kz2601.metamon.R
import jp.kz2601.metamon.auth.AuthSession
import kotlinx.coroutines.delay

/**
 * 匿名で最初のアプリを作った次の起動で 1 回だけ出す案内(#1316・iOS の `AccountLinkPromptHost`)。
 * 他のシート(音声・同意・チュートリアル・設定・変身一覧)と完成直後のお知らせが閉じてから、起動の画面が落ち着いた頃に出す。
 *
 * @param blocked 他のシート・お知らせが出ている・生成中
 */
@Composable
fun AccountLinkPromptHost(prompt: AccountLinkPrompt, session: AuthSession, blocked: Boolean) {
    val waiting by prompt.visible.collectAsStateWithLifecycle()
    val user by session.user.collectAsStateWithLifecycle()
    var shown by remember { mutableStateOf(false) }
    LaunchedEffect(waiting, blocked) {
        shown = false
        if (!waiting || blocked) return@LaunchedEffect
        delay(PROMPT_DELAY_MILLIS)
        shown = true
    }
    // 引き継いだ(匿名でなくなった)ら閉じる
    LaunchedEffect(user?.isAnonymous) {
        if (user?.isAnonymous == false) prompt.dismiss()
    }
    if (shown && waiting && user?.isAnonymous == true) {
        AccountLinkPromptSheet(session, onDismiss = prompt::dismiss)
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun AccountLinkPromptSheet(session: AuthSession, onDismiss: () -> Unit) {
    ModalBottomSheet(onDismissRequest = onDismiss, modifier = Modifier.testTag("accountLink.prompt")) {
        // 横向きは高さが足りず「あとで」が見切れるのでスクロールに入れる(iOS #625)
        Column(
            modifier = Modifier.fillMaxWidth().verticalScroll(rememberScrollState()).padding(bottom = 16.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            Icon(
                Icons.Filled.AccountCircle,
                contentDescription = null,
                tint = MaterialTheme.colorScheme.primary,
                modifier = Modifier.size(48.dp),
            )
            Text(
                stringResource(R.string.account_link_prompt_title),
                style = MaterialTheme.typography.titleLarge.copy(fontWeight = FontWeight.Bold),
                textAlign = TextAlign.Center,
                modifier = Modifier.padding(horizontal = 16.dp),
            )
            Text(
                stringResource(R.string.account_link_prompt_body),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                textAlign = TextAlign.Center,
                modifier = Modifier.padding(horizontal = 16.dp),
            )
            // 中身は設定の引き継ぎと同じ(Google / Apple で続ける)
            AccountLinkSection(session)
            TextButton(onClick = onDismiss, modifier = Modifier.testTag("accountLink.later")) {
                Text(stringResource(R.string.account_link_prompt_later))
            }
        }
    }
}

/** 起動直後の画面が落ち着いてから(iOS の `linkPromptDelay` 1.5 秒) */
private const val PROMPT_DELAY_MILLIS = 1_500L
