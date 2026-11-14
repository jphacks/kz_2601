package jp.kz2601.metamon.notifications

import jp.kz2601.metamon.dsl.Activity
import jp.kz2601.metamon.renderer.components.play.TimerRun

/** stat / progressBar を読んだ結果(iOS の `LiveActivityContent.StatReading`)。 */
data class ActivityStatReading(val label: String? = null, val value: String? = null, val progress: Double? = null)

/**
 * `timerDisplay.liveActivity` の 1 回分の状態(#913・iOS の `TimerActivityUpdate`)。
 * [key] は `<appId>/<画面>/<部品 id>`([TimerRunStore] と同じ)。[totalSeconds] は countdown の合計秒(stopwatch は null)。
 */
data class TimerActivityUpdate(
    val key: String,
    val appId: String,
    val appName: String,
    val label: String,
    val totalSeconds: Int?,
    val run: TimerRun,
)

/** 一時停止・終了の一言(言語は端末の設定・[ActivityNotifications] が入れる)。 */
data class TimerActivityLabels(val paused: String, val finished: String)

/** 進行中の通知の中身(iOS の `TailorActivityAttributes` + `ContentState`)。 */
data class ActivityContent(
    val appId: String,
    val appName: String,
    val title: String,
    val subtitle: String?,
    val clock: Clock,
    val value: String?,
    val valueLabel: String?,
    /** progressBar 由来の 0〜100(超過は 100 で頭打ち) */
    val progressPercent: Int?,
    /** この時刻に OS が消す(countdown の終わり・iOS が 8 時間で終えるのに合わせる) */
    val timeoutAtMillis: Long,
    /** timerDisplay.liveActivity 由来なら部品のキー(通知の tag を分ける)。startActivity は null */
    val timerKey: String? = null,
    /** 終わった表示(スワイプで消せる・少し残して消える) */
    val finished: Boolean = false,
) {
    sealed interface Clock {
        /** 残り時間(終わりに向けて数え下げる) */
        data class Countdown(val endsAtMillis: Long) : Clock

        /** 経過時間(since から数え上げる) */
        data class Elapsed(val sinceMillis: Long) : Clock

        /** 止まっている(OS の時計は使わず、本文に秒を書く) */
        data class Paused(val shownSeconds: Int) : Clock
    }

    /** 本文: subtitle と「ラベル 値」を行に分けて。 */
    val body: String?
        get() = listOfNotNull(subtitle, statLine).joinToString("\n").ifEmpty { null }

    val statLine: String?
        get() = listOfNotNull(valueLabel, value).joinToString(" ").ifEmpty { null }
}

/**
 * startActivity / endActivity と timerDisplay.liveActivity の Android 版(#1030・#913・iOS の Live Activity)。
 * 通知に触らない純粋関数なので JVM でテストする。
 * 表示は startActivity が 1 アプリにつき 1 つ(iOS の `app:<appId>`)、タイマーは部品ごと(iOS の `timer:<key>`)。通知の tag で見分ける。
 */
object ActivityNotificationPlanner {
    const val TAG_PREFIX = "activity:"

    /** タイマーの表示の tag(`activity:timer:<key>`) */
    const val TIMER_PREFIX = "timer:"

    /** iOS はライブアクティビティを 8 時間で終える(Go の dsl.MaxActivityMinutes と同じ) */
    const val MAX_MINUTES = 480

    /** 終わった表示を残す時間(iOS の `LiveActivityContent.finishedLinger`) */
    const val FINISHED_LINGER_MILLIS = 15 * 60 * 1000L

    fun tag(appId: String): String = TAG_PREFIX + appId

    fun timerTag(key: String): String = TAG_PREFIX + TIMER_PREFIX + key

    fun content(spec: Activity, appId: String, appName: String, stat: ActivityStatReading?, nowMillis: Long): ActivityContent {
        val minutes = spec.minutes?.takeIf { it > 0 }?.coerceAtMost(MAX_MINUTES)
        val clock = if (minutes != null) {
            ActivityContent.Clock.Countdown(nowMillis + minutes * MINUTE)
        } else {
            ActivityContent.Clock.Elapsed(nowMillis)
        }
        return ActivityContent(
            appId = appId,
            appName = appName,
            title = spec.title,
            subtitle = spec.subtitle?.takeIf { it.isNotBlank() },
            clock = clock,
            value = stat?.value,
            valueLabel = stat?.label?.takeIf { it.isNotBlank() },
            progressPercent = stat?.progress?.let { (it.coerceIn(0.0, 1.0) * 100).toInt() },
            timeoutAtMillis = nowMillis + (minutes ?: MAX_MINUTES) * MINUTE,
        )
    }

    /** 出す。同じアプリの表示が出ていれば差し替える(音・振動をくり返さない)。 */
    data class Post(val tag: String, val content: ActivityContent, val update: Boolean)

    fun start(content: ActivityContent, active: Set<String>): Post {
        val tag = content.timerKey?.let(::timerTag) ?: tag(content.appId)
        return Post(tag, content, update = tag in active)
    }

    /** endActivity・アプリの削除: そのアプリの表示の tag(出ていなければ空)。 */
    fun end(appId: String, active: Set<String>): List<String> = listOfNotNull(tag(appId).takeIf { it in active })

    /** アプリを消した時: startActivity の表示とそのアプリのタイマーの表示を全部。 */
    fun forApp(appId: String, active: Set<String>): List<String> {
        val timers = timerTag(TimerRunStore.appPrefix(appId))
        return active.filter { it == tag(appId) || it.startsWith(timers) }.sorted()
    }

    /** サインアウト・別の人(LocalDataWipe): 進行中の表示を全部。 */
    fun wipe(active: Set<String>): List<String> = active.filter { it.startsWith(TAG_PREFIX) }.sorted()

    // MARK: - timerDisplay.liveActivity

    /** タイマーの表示をどうするか(iOS の `LiveActivityContent.Plan`)。 */
    sealed interface TimerPlan {
        /** 出す / 差し替える */
        data class Show(val content: ActivityContent) : TimerPlan

        /** 「終わりました」を少し残して消す */
        data class Finish(val content: ActivityContent) : TimerPlan

        /** すぐ消す(リセット・まだ始めていない) */
        data object End : TimerPlan
    }

    /**
     * 走行状態 → 表示(iOS の `LiveActivityContent.plan(for:)`)。
     * 走っている間は OS の時計が描くので、アプリが止まっていても残り時間が進む。
     */
    fun timerPlan(update: TimerActivityUpdate, labels: TimerActivityLabels, nowMillis: Long): TimerPlan {
        val run = update.run
        val total = update.totalSeconds?.takeIf { it > 0 }
        val base = ActivityContent(
            appId = update.appId,
            appName = update.appName,
            title = update.label,
            subtitle = null,
            clock = ActivityContent.Clock.Elapsed(nowMillis),
            value = null,
            valueLabel = null,
            progressPercent = null,
            timeoutAtMillis = nowMillis + MAX_MINUTES * MINUTE,
            timerKey = update.key,
        )
        if (run.isRunning) {
            val endsAt = total?.let { run.endsAtMillis(it) }
            val shownStart = run.startedAtMillis!! - run.base * 1000L
            return TimerPlan.Show(
                base.copy(
                    clock = if (endsAt != null) ActivityContent.Clock.Countdown(endsAt) else ActivityContent.Clock.Elapsed(shownStart),
                    progressPercent = total?.let { percent(run.elapsed(nowMillis, it), it) },
                    // 0 になった後も少し残す(前面に戻った時に「終わりました」へ差し替わる)
                    timeoutAtMillis = endsAt?.plus(FINISHED_LINGER_MILLIS) ?: base.timeoutAtMillis,
                ),
            )
        }
        if (run.base == 0) return TimerPlan.End
        if (total != null && run.base >= total) {
            return TimerPlan.Finish(
                base.copy(
                    subtitle = labels.finished,
                    clock = ActivityContent.Clock.Paused(0),
                    value = TimerRun.format(0),
                    progressPercent = 100,
                    timeoutAtMillis = nowMillis + FINISHED_LINGER_MILLIS,
                    finished = true,
                ),
            )
        }
        val shown = total?.let { it - run.base } ?: run.base
        return TimerPlan.Show(
            base.copy(
                subtitle = labels.paused,
                clock = ActivityContent.Clock.Paused(shown),
                value = TimerRun.format(shown),
                progressPercent = total?.let { percent(run.base, it) },
            ),
        )
    }

    private fun percent(elapsed: Int, total: Int): Int = ((elapsed.toLong() * 100) / total).toInt().coerceIn(0, 100)

    private const val MINUTE = 60_000L
}
