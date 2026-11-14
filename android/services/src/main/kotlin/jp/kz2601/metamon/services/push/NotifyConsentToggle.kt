package jp.kz2601.metamon.services.push

/**
 * 設定の「他の人の行動で届く通知」の切り替え(#339・iOS の `PushConsentSection`)。
 * 楽観的 UI: 先に表示を変え、送れなければ元に戻して [failed] を立てる。
 */
class NotifyConsentToggle(
    private val client: PushDeviceClient,
    private val store: NotifyConsentStore,
    private val log: (String) -> Unit = {},
) {
    data class State(val isOn: Boolean, val failed: Boolean = false)

    fun initial(uid: String?, appId: String): State = State(isOn = uid != null && store.allowed(uid, appId))

    /**
     * [onOptimistic] で先に新しい値を出し、結果の状態を返す。
     * サインアウト中は送らずに元へ戻す。
     */
    suspend fun change(allowed: Boolean, uid: String?, appId: String, onOptimistic: (State) -> Unit): State {
        onOptimistic(State(isOn = allowed))
        if (uid == null) return State(isOn = !allowed, failed = true)
        return try {
            client.setNotifyConsent(appId, allowed)
            store.set(allowed, uid, appId)
            State(isOn = allowed)
        } catch (e: kotlin.coroutines.cancellation.CancellationException) {
            throw e
        } catch (e: Exception) {
            log("push: notify consent not saved appId=$appId allowed=$allowed (${e.javaClass.simpleName})")
            State(isOn = !allowed, failed = true)
        }
    }
}
