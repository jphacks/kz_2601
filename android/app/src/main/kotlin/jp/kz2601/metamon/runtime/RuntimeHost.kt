package jp.kz2601.metamon.runtime

import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.Activity
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.Notification
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.renderer.ActionContext

/** 端末の機能がまだ無い(Android で未実装)。AppRuntime が決まったトーストにする。 */
class HostUnavailableException(val type: ActionType) : Exception("unavailable: ${type.wire}")

/**
 * 端末の機能が要るアクションの受け口(iOS は AppRuntime の拡張と Services が持つ)。
 * 実装は機能ごとの Issue で入れる。既定は [HostUnavailableException] を投げる = 「まだ使えません」のトースト。
 */
interface RuntimeHost {
    /** 外部の URL を開く(許可リストは AppRuntime が先に見る)。開けたら true。 */
    suspend fun openUrl(url: String): Boolean = throw HostUnavailableException(ActionType.OPEN_URL)

    /** 通知を予約する。次に鳴る時の言い方(「明日 9:00」など)、予約できなければ null。 */
    suspend fun scheduleNotification(spec: Notification, appId: String): String? =
        throw HostUnavailableException(ActionType.SCHEDULE_NOTIFICATION)

    suspend fun cancelNotification(id: String, appId: String): Unit = throw HostUnavailableException(ActionType.CANCEL_NOTIFICATION)

    /**
     * DSL の読み込み・記録の保存・削除・同期の後(まとめて 1 回)。`app.notifications[]` の予約し直しと条件の通知の評価(#886)。
     * 既定は何もしない(通知の無いホスト)。
     */
    suspend fun notificationsChanged(appId: String, dsl: AppDSL, records: Map<String, List<Record>>) {}

    /** 記録を書き出して共有シートに渡す(#76)。 */
    suspend fun export(action: Action, context: ActionContext, runtime: AppRuntime): Unit = throw HostUnavailableException(ActionType.EXPORT)

    /** カレンダーに予定を足す画面を開く。 */
    suspend fun addToCalendar(action: Action, context: ActionContext, runtime: AppRuntime): Unit =
        throw HostUnavailableException(ActionType.ADD_TO_CALENDAR)

    /** ISBN から本を引いてフォームに入れる。 */
    suspend fun lookupBook(action: Action, context: ActionContext, runtime: AppRuntime): Unit =
        throw HostUnavailableException(ActionType.LOOKUP_BOOK)

    /**
     * timerDisplay の終了通知(iOS の TimerNotifier)。[endsAtEpochMillis] にその時刻で予約、null なら取り消す。
     * 既定は何もしない(タイマーは前面で動く)。端末の通知の予約は子 Issue で入れる。
     */
    suspend fun timerChanged(key: String, appName: String, timerName: String, endsAtEpochMillis: Long?): Unit = Unit

    /** 進行中の表示(iOS の Live Activity。Android は常駐通知を想定・#78)。 */
    suspend fun startActivity(spec: Activity, appId: String): Unit = throw HostUnavailableException(ActionType.START_ACTIVITY)

    suspend fun endActivity(appId: String): Unit = throw HostUnavailableException(ActionType.END_ACTIVITY)
}

/** 何も持たないホスト(テスト・未接続)。 */
object UnavailableHost : RuntimeHost
