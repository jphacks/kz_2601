package jp.kz2601.metamon.runtime

import jp.kz2601.metamon.dsl.ai.AiTextError
import jp.kz2601.metamon.dsl.ai.AiTextRequest
import jp.kz2601.metamon.dsl.ai.AiTextResult
import jp.kz2601.metamon.renderer.components.ai.AiButtonMetric
import jp.kz2601.metamon.renderer.components.ai.AiHost
import jp.kz2601.metamon.renderer.components.ai.OnDeviceAi
import jp.kz2601.metamon.renderer.components.ai.UnsupportedOnDeviceAi
import jp.kz2601.metamon.services.AIClient
import jp.kz2601.metamon.services.AiTextGate

/** オンライン AI を使ってよいかの材料(同意・Pro・課金の制限が有効か)。 */
data class AiTextAccess(val consentGranted: Boolean, val isPro: Boolean, val enforced: Boolean)

/**
 * aiButton のホスト側(iOS の AppRuntime+AIText / +AIButtonMetrics)。
 * 端末内 AI が使えない時だけ `POST /v1/text` を呼ぶ。判定は [AiTextGate](同意 → Pro)。断る時は理由の [AiTextError] を投げ、部品が文言を出す。
 * 同意が無い時は [onNeedsConsent] で同意の画面を出す(iOS の `requireAIConsent`・#888)。
 * Pro が要る時(端末の判定・サーバーの 402)は [onNeedsPro] で課金画面を出す(iOS の `requestProForAIText`)。
 */
class AppAiHost(
    private val ai: AIClient,
    private val appId: () -> String,
    private val access: () -> AiTextAccess,
    /** 共有の記録を持つアプリか(参加者は作成者の Pro で使える・判定はサーバー) */
    private val sharedApp: () -> Boolean = { false },
    override val isMock: Boolean = false,
    override val onDevice: OnDeviceAi = UnsupportedOnDeviceAi,
    /** 端末内 AI の集計の受け口(貯めて 1 日 1 回送るのは `AiButtonMetricsRecorder`・#888) */
    private val metrics: (AiButtonMetric) -> Unit = {},
    /** 同意が要る時に同意の画面を出す(#888) */
    private val onNeedsConsent: () -> Unit = {},
    /** Pro が要る時に課金画面を出す。引数は aiChat(会話)か */
    private val onNeedsPro: (chat: Boolean) -> Unit = {},
) : AiHost {
    override val supportsOnlineText: Boolean get() = true

    override suspend fun generateText(request: AiTextRequest): AiTextResult {
        val a = access()
        return when (AiTextGate.decide(a.consentGranted, a.isPro, a.enforced, sharedApp())) {
            AiTextGate.Decision.NEEDS_CONSENT -> {
                onNeedsConsent()
                throw AiTextError.NeedsConsent
            }
            AiTextGate.Decision.NEEDS_PRO -> {
                onNeedsPro(request.isChat)
                throw AiTextError.NeedsPro
            }
            AiTextGate.Decision.SEND -> try {
                ai.text(request.inApp(appId()))
            } catch (e: AiTextError.NeedsPro) {
                // サーバーが 402 pro_required(端末の Pro の判定が古いなど)。作成者の Pro が要る時(CreatorNeedsPro)は画面を出さない
                onNeedsPro(request.isChat)
                throw e
            }
        }
    }

    override fun record(event: AiButtonMetric) = metrics(event)
}
