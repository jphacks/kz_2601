package jp.kz2601.metamon.desktop.feedback

/** `PUT /v1/me/notify-consent/{appId}` を送る口(iOS の `PushDeviceClient.setNotifyConsent`)。試験では偽物に差し替える。 */
fun interface NotifyConsentApi {
    suspend fun setNotifyConsent(appId: String, allowed: Boolean)
}

/**
 * フィードバックの「直ったら知らせてほしい」(既定 off・#3004。iOS の `FeedbackFixedOptIn` と同じ)。
 * on にして送った時だけ `PUT /v1/me/notify-consent/feedback-fixed` を送る。off の時は何も送らない。
 * 知らせは起動中のトレイ / OS の通知で出る(アプリを閉じている間は出ない)。
 */
class FeedbackFixedOptIn(private val api: NotifyConsentApi) {
    /** フィードバック本体の送信が成功した後に呼ぶ。同意の送信に失敗してもフィードバックは成功のまま(false を返すだけ)。 */
    suspend fun afterSend(optedIn: Boolean): Boolean {
        if (!optedIn) return true
        return runCatching { api.setNotifyConsent(APP_ID, true) }.isSuccess
    }

    companion object {
        const val APP_ID = "feedback-fixed"
    }
}
