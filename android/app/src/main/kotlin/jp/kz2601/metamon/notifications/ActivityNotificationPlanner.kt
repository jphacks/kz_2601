package jp.kz2601.metamon.notifications

import jp.kz2601.metamon.dsl.Activity

/** stat / progressBar を読んだ結果(iOS の `LiveActivityContent.StatReading`)。 */
data class ActivityStatReading(val label: String? = null, val value: String? = null, val progress: Double? = null)

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
) {
    sealed interface Clock {
        /** 残り時間(終わりに向けて数え下げる) */
        data class Countdown(val endsAtMillis: Long) : Clock

        /** 経過時間(since から数え上げる) */
        data class Elapsed(val sinceMillis: Long) : Clock
    }

    /** 本文: subtitle と「ラベル 値」を行に分けて。 */
    val body: String?
        get() = listOfNotNull(subtitle, statLine).joinToString("\n").ifEmpty { null }

    val statLine: String?
        get() = listOfNotNull(valueLabel, value).joinToString(" ").ifEmpty { null }
}

/**
 * startActivity / endActivity の Android 版(#1030・iOS の Live Activity)。通知に触らない純粋関数なので JVM でテストする。
 * 表示は 1 アプリにつき 1 つ(iOS の `app:<appId>` キー)。通知の tag で見分ける。
 */
object ActivityNotificationPlanner {
    const val TAG_PREFIX = "activity:"

    /** iOS はライブアクティビティを 8 時間で終える(Go の dsl.MaxActivityMinutes と同じ) */
    const val MAX_MINUTES = 480

    fun tag(appId: String): String = TAG_PREFIX + appId

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
        val tag = tag(content.appId)
        return Post(tag, content, update = tag in active)
    }

    /** endActivity・アプリの削除: そのアプリの表示の tag(出ていなければ空)。 */
    fun end(appId: String, active: Set<String>): List<String> = listOfNotNull(tag(appId).takeIf { it in active })

    /** サインアウト・別の人(LocalDataWipe): 進行中の表示を全部。 */
    fun wipe(active: Set<String>): List<String> = active.filter { it.startsWith(TAG_PREFIX) }.sorted()

    private const val MINUTE = 60_000L
}
