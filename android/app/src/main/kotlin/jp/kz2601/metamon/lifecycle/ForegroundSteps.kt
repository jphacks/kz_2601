package jp.kz2601.metamon.lifecycle

import jp.kz2601.metamon.AppContainer

/**
 * 前面・背面で回す処理の一覧(#891)。前面はこの順、背面は逆順。
 * 1. 記録と DSL: 送信待ち → 購読 → 取り直し → 通知の予約(前面) / 削除の確定・購読を止める(背面)
 * 2. 端末の通知の登録(#887・通知の許可を設定で変えて戻った時もここ)
 * 3. aiButton の前の日までの集計(#888)
 * 4. マイク: 背面で文字起こしの録音を捨てる(音声の画面は画面側の ON_STOP で止める)
 */
fun AppContainer.foregroundSteps(): List<ForegroundStep> = listOf(
    object : ForegroundStep {
        override val name = "runtime"
        override suspend fun foreground() = runtime.setForeground(true)
        override suspend fun background() = runtime.setForeground(false)
    },
    object : ForegroundStep {
        override val name = "push"
        override suspend fun foreground() {
            push?.refresh()
        }
    },
    object : ForegroundStep {
        override val name = "ai-button-metrics"
        override suspend fun foreground() = flushAiButtonMetrics()
    },
    object : ForegroundStep {
        override val name = "microphone"
        override suspend fun background() = transcriber.cancelRecording()
    },
)
