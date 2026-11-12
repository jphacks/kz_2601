package jp.kz2601.metamon.desktop.usage

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import jp.kz2601.metamon.services.PlanUsageSource
import kotlinx.coroutines.CancellationException
import java.time.Instant

/**
 * 設定の「使っている量」を読む(#2814)。開くたびに [refresh]。
 * 初めて読む間は [State.Loading](スケルトン)、読めなかった時は前回の値を残して [State.Failed]。
 */
class UsageLoader(private val source: PlanUsageSource, private val clock: () -> Instant = Instant::now) {
    sealed interface State {
        data object Loading : State

        /** null = サーバーに無い(404)・数が無い。節を出さない */
        data class Loaded(val summary: UsageText.Summary?) : State

        /** 読めなかった。[last] は前回読めた値(無ければ null) */
        data class Failed(val last: UsageText.Summary?) : State
    }

    var state: State by mutableStateOf(State.Loading)
        private set

    private var last: UsageText.Summary? = null
    private var loaded = false

    suspend fun refresh() {
        if (!loaded) state = State.Loading
        state = try {
            val summary = source.myUsage()?.let { UsageText.of(it, clock()) }
            last = summary
            loaded = true
            State.Loaded(summary)
        } catch (e: CancellationException) {
            throw e
        } catch (_: Throwable) {
            State.Failed(last)
        }
    }
}
