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
 * 同意画面(#053)・課金画面(#073)はまだ無いので、今は出さずに理由だけ返す。
 */
class AppAiHost(
    private val ai: AIClient,
    private val appId: () -> String,
    private val access: () -> AiTextAccess,
    /** 共有の記録を持つアプリか(参加者は作成者の Pro で使える・判定はサーバー) */
    private val sharedApp: () -> Boolean = { false },
    override val isMock: Boolean = false,
    override val onDevice: OnDeviceAi = UnsupportedOnDeviceAi,
    /** 端末内 AI の集計の受け口(1 日 1 回送るのは #053 の集計の口が入ってから) */
    private val metrics: (AiButtonMetric) -> Unit = {},
) : AiHost {
    override val supportsOnlineText: Boolean get() = true

    override suspend fun generateText(request: AiTextRequest): AiTextResult {
        val a = access()
        return when (AiTextGate.decide(a.consentGranted, a.isPro, a.enforced, sharedApp())) {
            AiTextGate.Decision.NEEDS_CONSENT -> throw AiTextError.NeedsConsent
            AiTextGate.Decision.NEEDS_PRO -> throw AiTextError.NeedsPro
            AiTextGate.Decision.SEND -> ai.text(request.inApp(appId()))
        }
    }

    override fun record(event: AiButtonMetric) = metrics(event)
}
