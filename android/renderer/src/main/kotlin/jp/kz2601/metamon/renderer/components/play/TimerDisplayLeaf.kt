package jp.kz2601.metamon.renderer.components.play

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.Saver
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.LocalLifecycleOwner
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentMode
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.renderer.R
import jp.kz2601.metamon.renderer.registry.RenderContext
import jp.kz2601.metamon.renderer.theme.RenderTheme
import jp.kz2601.metamon.renderer.theme.colors
import jp.kz2601.metamon.renderer.theme.renderCard
import kotlinx.coroutines.delay

/**
 * 走っている時間(iOS の `TimerRun`)。経過は開始時刻から都度計算するので、背面に回っても正しい時間を出す。
 * @param base 一時停止までに積み上がった経過秒
 * @param startedAtMillis 走行中なら開始(再開)時刻。停止中は null
 */
data class TimerRun(val base: Int = 0, val startedAtMillis: Long? = null) {
    val isRunning: Boolean get() = startedAtMillis != null

    /** 経過秒(countdown は [total] で頭打ち)。 */
    fun elapsed(nowMillis: Long, total: Int? = null): Int {
        val raw = base + (startedAtMillis?.let { maxOf(((nowMillis - it) / 1000).toInt(), 0) } ?: 0)
        return total?.let { minOf(raw, it) } ?: raw
    }

    /** countdown が走行中なら終了時刻。停止中は null(通知は予約しない)。 */
    fun endsAtMillis(total: Int): Long? = startedAtMillis?.let { it + (total - base) * 1000L }

    companion object {
        val saver: Saver<TimerRun, List<Long>> = Saver(
            save = { listOf(it.base.toLong(), it.startedAtMillis ?: -1L) },
            restore = { TimerRun(it[0].toInt(), it[1].takeIf { s -> s >= 0 }) },
        )

        /** 「05:03」、1 時間を超えたら「1:05:03」。 */
        fun format(seconds: Int): String {
            val h = seconds / 3600
            val m = (seconds % 3600) / 60
            val s = seconds % 60
            return if (h > 0) "%d:%02d:%02d".format(h, m, s) else "%02d:%02d".format(m, s)
        }

        /** 終了通知の予約時刻(countdown が走っていて、まだ先の時だけ)。null なら取り消す。 */
        fun notificationAt(isCountdown: Boolean, run: TimerRun, total: Int, nowMillis: Long): Long? {
            if (!isCountdown) return null
            val end = run.endsAtMillis(total) ?: return null
            return end.takeIf { it > nowMillis }
        }
    }
}

/**
 * `timerDisplay`: ストップウォッチ(mode=stopwatch・カウントアップ)/ タイマー(mode=countdown・`seconds` から 0 へ)。
 * 開始 / 一時停止 / リセット。一時停止・終了時に経過秒を `field` へ書く(フォーム画面のみ)。iOS の TimerDisplayLeaf。
 *
 * 走行状態は画面の保存状態(回転・背面)と runtime の保存([TimerHost.saveTimer]・アプリを終了しても残る)に持ち、経過は開始時刻から出し直す。
 * countdown の終了通知は runtime が [TimerHost] なら予約を頼む。前面で見えている間はバナーを出さない。
 */
@Composable
fun TimerDisplayLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) {
    val isCountdown = c.mode == ComponentMode.COUNTDOWN
    val total = maxOf(c.seconds ?: 0, 0)
    val key = "${ctx.route.screen}/${c.id.orEmpty()}"
    val host = ctx.runtime as? TimerHost
    var run by rememberSaveable(key, stateSaver = TimerRun.saver) { mutableStateOf(host?.loadTimer(key) ?: TimerRun()) }
    var elapsed by rememberSaveable(key) { mutableIntStateOf(run.elapsed(System.currentTimeMillis(), if (isCountdown) total else null)) }
    val shown = if (isCountdown) maxOf(total - elapsed, 0) else elapsed
    val finished = isCountdown && elapsed >= total
    val name = c.label?.takeIf { it.isNotEmpty() } ?: stringResource(R.string.play_timer_default_name)

    // アプリを終了しても残す(次に開いた時に残り時間を出し直す・#1128)
    // liveActivity なら進行中の表示にも映す(開始・一時停止・リセット・終了のどれでもここを通る・#913)
    LaunchedEffect(run, key) {
        host?.saveTimer(key, run)
        if (c.liveActivity == true) host?.timerActivity(key, name, if (isCountdown) total else null, run)
    }
    val lifecycle = LocalLifecycleOwner.current.lifecycle
    DisposableEffect(key, lifecycle, host) {
        var visible = false
        val observer = LifecycleEventObserver { _, _ ->
            val now = lifecycle.currentState.isAtLeast(Lifecycle.State.RESUMED)
            if (now != visible) {
                visible = now
                host?.timerVisible(key, now)
            }
        }
        lifecycle.addObserver(observer)
        onDispose {
            lifecycle.removeObserver(observer)
            if (visible) host?.timerVisible(key, false)
        }
    }

    fun notify(next: TimerRun) {
        host?.timerChanged(key, name, TimerRun.notificationAt(isCountdown, next, total, System.currentTimeMillis()))
    }

    fun save() {
        val field = c.field ?: return
        ctx.form?.values?.set(field, JSONValue.Num(elapsed.toDouble()))
    }

    LaunchedEffect(run, key) {
        while (run.isRunning) {
            elapsed = run.elapsed(System.currentTimeMillis(), if (isCountdown) total else null)
            if (isCountdown && elapsed >= total) {
                run = TimerRun(base = total)
                notify(run)
                save()
                break
            }
            delay(250)
        }
    }

    Column(
        modifier.renderCard(),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        c.label?.takeIf { it.isNotEmpty() }?.let {
            Text(it, style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
        Text(
            TimerRun.format(shown),
            Modifier.semantics { if (finished) liveRegion = LiveRegionMode.Polite },
            style = MaterialTheme.typography.displayMedium.copy(
                fontSize = 44.sp,
                fontWeight = FontWeight.SemiBold,
                fontFeatureSettings = "tnum",
            ),
            color = if (finished) RenderTheme.colors.negative else RenderTheme.colors.brandA,
        )
        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(12.dp)) {
            val shape = RoundedCornerShape(RenderTheme.Radius.button)
            Button(
                onClick = {
                    val now = System.currentTimeMillis()
                    if (run.isRunning) {
                        elapsed = run.elapsed(now, if (isCountdown) total else null)
                        run = TimerRun(base = elapsed)
                        save()
                    } else {
                        run = TimerRun(base = elapsed, startedAtMillis = now)
                    }
                    notify(run)
                },
                enabled = !finished,
                modifier = Modifier.weight(1f),
                shape = shape,
            ) {
                val label = when {
                    run.isRunning -> R.string.play_timer_pause
                    elapsed > 0 && !finished -> R.string.play_timer_resume
                    else -> R.string.play_timer_start
                }
                Text(stringResource(label), fontWeight = FontWeight.SemiBold)
            }
            OutlinedButton(
                onClick = {
                    run = TimerRun()
                    elapsed = 0
                    notify(run)
                },
                enabled = elapsed > 0 || run.isRunning,
                modifier = Modifier.weight(1f),
                shape = shape,
            ) {
                Text(stringResource(R.string.play_timer_reset))
            }
        }
    }
}
