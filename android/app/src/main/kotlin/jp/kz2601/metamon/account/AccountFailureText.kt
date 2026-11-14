package jp.kz2601.metamon.account

import jp.kz2601.metamon.R
import jp.kz2601.metamon.runtime.UiText
import jp.kz2601.metamon.services.AuthError

/**
 * アカウントの削除・ログアウトの失敗ダイアログの本文(iOS の `SettingsSheet` の alert)。
 * 例外の文言(Firebase の英語・[AuthError.Unsupported] の開発者向けの文)は出さない。
 */
internal object AccountFailureText {
    val TRY_AGAIN: UiText = UiText.Res(R.string.account_failed_try_again)
    val REQUIRES_RECENT_LOGIN: UiText = UiText.Res(R.string.account_delete_requires_recent_login)

    fun of(e: Exception): UiText = when (e) {
        is AuthError.RequiresRecentLogin -> REQUIRES_RECENT_LOGIN
        // Unavailable はアプリが strings から作った文言なのでそのまま
        is AuthError.Unavailable -> e.message?.takeIf { it.isNotBlank() }?.let(UiText::Raw) ?: TRY_AGAIN
        else -> TRY_AGAIN
    }
}
