package jp.kz2601.metamon.renderer.components.ai

import jp.kz2601.metamon.dsl.ai.AiTextError
import jp.kz2601.metamon.dsl.ai.AiTextRequest
import jp.kz2601.metamon.dsl.ai.AiTextResult

/**
 * aiButton がホストに頼むこと(iOS の `AITextHost`)。持たないホスト(プレビュー・テスト)は端末内 AI だけ・数えない。
 * :renderer は :services を知らないので、オンライン版(`POST /v1/text`)と同意・Pro の確認は :app が持つ。
 */
interface AiHost {
    /** 端末内 AI。既定は「この端末では使えない」。 */
    val onDevice: OnDeviceAi get() = UnsupportedOnDeviceAi

    /** オンライン版を持つか。 */
    val supportsOnlineText: Boolean

    /** モックの構成(端末内 AI より決まった文のオンライン版を使う)。 */
    val isMock: Boolean get() = false

    /** オンライン版。端末内 AI が使えない時だけ呼ぶ。同意・Pro の確認もホストが持ち、断る時は [AiTextError] を投げる。 */
    suspend fun generateText(request: AiTextRequest): AiTextResult

    /** 端末内 AI の集計(#446)。決まったラベルだけ渡す。送るか・いつ送るかはホストが決める。 */
    fun record(event: AiButtonMetric) = Unit
}

/** 端末内 AI の集計の 1 件(入力の文は持たない)。 */
sealed interface AiButtonMetric {
    /** ボタンを出した時の可用性 */
    data class Shown(val availability: String) : AiButtonMetric

    /** 端末内 AI で動かした(failure = null は成功) */
    data class Ran(val task: String, val failure: String?) : AiButtonMetric
}

/**
 * 端末内 AI(iOS の Foundation Models に当たるもの)。Android は Gemini Nano(ML Kit GenAI・対応端末だけ)を想定し、
 * 実装は :app が差し込む。描画器は可用性を見て、使えなければオンライン版に回すか理由を出す。
 */
interface OnDeviceAi {
    fun availability(): OnDeviceAvailability

    /** 要約・分類など。答え(整える前)を返す。 */
    suspend fun run(request: AiOnDeviceRequest): String

    /** generate。 */
    suspend fun generate(request: AiTextRequest): AiTextResult
}

/** 端末内 AI を持たない(既定)。 */
object UnsupportedOnDeviceAi : OnDeviceAi {
    override fun availability(): OnDeviceAvailability = OnDeviceAvailability.Unavailable(OnDeviceAvailability.Reason.DEVICE_NOT_ELIGIBLE)

    override suspend fun run(request: AiOnDeviceRequest): String = throw AiTextError.Unsupported

    override suspend fun generate(request: AiTextRequest): AiTextResult = throw AiTextError.Unsupported
}
