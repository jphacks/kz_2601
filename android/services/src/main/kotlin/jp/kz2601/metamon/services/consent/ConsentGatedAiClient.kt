package jp.kz2601.metamon.services.consent

import jp.kz2601.metamon.dsl.ai.AiTextError
import jp.kz2601.metamon.dsl.ai.AiTextRequest
import jp.kz2601.metamon.dsl.ai.AiTextResult
import jp.kz2601.metamon.services.AIClient
import jp.kz2601.metamon.services.ChatMessage
import jp.kz2601.metamon.services.ChatMode
import jp.kz2601.metamon.services.ChatTurn
import jp.kz2601.metamon.services.GenerateMode
import jp.kz2601.metamon.services.GenerateProgress
import jp.kz2601.metamon.services.MorphResult

/** AI への送信に同意していない(生成・対話)。aiButton は [AiTextError.NeedsConsent]。 */
class AiConsentRequired : IllegalStateException("AI consent not granted")

/**
 * 同意が無ければ ai サービスへ一切送らない包み(iOS の `ConsentGatedAIClient`・最後の砦)。
 * 画面側でも送る前に止めて同意の画面を出すが、別の入口から呼ばれても送らないよう `ai` をこれで包む。
 */
class ConsentGatedAiClient(
    private val base: AIClient,
    private val isGranted: () -> Boolean,
) : AIClient {
    override suspend fun morph(transcript: String, appId: String, currentVersion: Int, locale: String?, mode: GenerateMode): MorphResult {
        ensure()
        return base.morph(transcript, appId, currentVersion, locale, mode)
    }

    override suspend fun morphStreaming(
        transcript: String,
        appId: String,
        currentVersion: Int,
        locale: String?,
        mode: GenerateMode,
        progress: (GenerateProgress) -> Unit,
    ): MorphResult {
        ensure()
        return base.morphStreaming(transcript, appId, currentVersion, locale, mode, progress)
    }

    override suspend fun chat(appId: String, messages: List<ChatMessage>, locale: String?, mode: ChatMode?): ChatTurn {
        ensure()
        return base.chat(appId, messages, locale, mode)
    }

    override suspend fun resetChat(appId: String) {
        if (isGranted()) base.resetChat(appId)
    }

    override suspend fun text(request: AiTextRequest): AiTextResult {
        if (!isGranted()) throw AiTextError.NeedsConsent
        return base.text(request)
    }

    private fun ensure() {
        if (!isGranted()) throw AiConsentRequired()
    }
}
