package jp.kz2601.metamon.lifecycle

import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock

/** 前面・背面で動かす 1 つの処理。[foreground] / [background] の片方だけでもよい。 */
interface ForegroundStep {
    val name: String
    suspend fun foreground() {}
    suspend fun background() {}
}

/**
 * アプリの前面・背面の処理を 1 か所にまとめる(iOS の scenePhase の `.active` / `.background`・#891)。
 * 受け口は `ProcessLifecycleOwner` の onStart / onStop([MetamonApplication] が呼ぶ)。
 *
 * - 前面は [steps] の順、背面は逆順(後から立ち上げたものを先に止める)
 * - 同じ向きが続けて来ても 1 回だけ。前面 → 背面と素早く続いても、前の分が終わってから次を回す
 * - どれかが失敗しても残りは回す(記録は [log])
 */
class AppForeground(
    private val scope: CoroutineScope,
    private val steps: List<ForegroundStep>,
    private val log: (String) -> Unit = {},
) {
    private val lock = Mutex()
    private var foreground: Boolean? = null

    /** 組み立てた時は前面(最初の画面の起動時)。最初の onStart では前面の処理を重ねない。 */
    fun markForeground() {
        foreground = true
    }

    fun setForeground(active: Boolean) {
        if (foreground == active) return
        foreground = active
        scope.launch {
            lock.withLock {
                val ordered = if (active) steps else steps.asReversed()
                for (step in ordered) {
                    try {
                        if (active) step.foreground() else step.background()
                    } catch (e: CancellationException) {
                        throw e
                    } catch (e: Exception) {
                        log("lifecycle: ${step.name} ${if (active) "foreground" else "background"} failed (${e.javaClass.simpleName})")
                    }
                }
            }
        }
    }
}
