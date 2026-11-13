package jp.kz2601.metamon.desktop.chat

import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.services.AIClient
import jp.kz2601.metamon.services.ChatMode
import java.util.UUID

/** 「新しいアプリを作る」の会話の入口(#2160)。AI の接続先がある時だけ作る。 */
object ChatEntry {
    /** 新しいアプリの id(会話と作るで同じ物を使う)。 */
    fun newAppId(): String = UUID.randomUUID().toString().lowercase()

    /** 会話を作る口。[ai] が null(モック・接続先なし)なら null で、パレットに出さない。 */
    fun factory(ai: AIClient?, appId: () -> String = ::newAppId): (() -> ChatSession)? = ai?.let { client ->
        { ChatSession(client, appId(), ChatMode.CREATE, locale = { Strings.locale.language }) }
    }
}
