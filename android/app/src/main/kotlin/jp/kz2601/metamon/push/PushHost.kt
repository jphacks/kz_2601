package jp.kz2601.metamon.push

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.Scope
import jp.kz2601.metamon.services.push.NotifyConsentStore
import jp.kz2601.metamon.services.push.NotifyConsentToggle
import jp.kz2601.metamon.services.push.PushDeviceRegistrar
import jp.kz2601.metamon.services.sharing.AppMembersStore
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

/**
 * 他の人の行動で届く通知の窓口(#887・iOS の `PushHost`)。Android の部品は差し込みで受けるので JVM でテストできる。
 * - 許可済み(通知を出せる)の時だけ FCM のトークンを求める。確認のダイアログはここでは出さない(#886 の窓口が聞く)
 * - [registerDevices] が false ならトークンを取らず送らない(既定は [jp.kz2601.metamon.AppContainer.PUSH_DEVICE_REGISTRATION])
 * - モックの構成では作らない(AppContainer の push が null)
 */
class PushHost(
    private val registrar: PushDeviceRegistrar,
    private val consentStore: NotifyConsentStore,
    val consent: NotifyConsentToggle,
    private val uid: () -> String?,
    private val canPost: () -> Boolean,
    private val fetchToken: suspend () -> String?,
    private val registerDevices: Boolean,
    private val log: (String) -> Unit = {},
) {
    /** サインイン・前面復帰で呼ぶ(通知の許可の画面から戻った時もここを通る)。 */
    suspend fun refresh() {
        if (!registerDevices) return
        val uid = uid() ?: return
        if (!canPost()) {
            registrar.authorizationRevoked()
            return
        }
        val token = runCatching { fetchToken() }.onFailure { log("push: token not available (${it.javaClass.simpleName})") }.getOrNull() ?: return
        registrar.register(token, uid)
    }

    /** FCM のトークンが変わった(`onNewToken`)。 */
    suspend fun receive(token: String) {
        if (!registerDevices) return
        val uid = uid() ?: return
        if (canPost()) registrar.register(token, uid)
    }

    /** サインアウト・退会の前に呼ぶ(サーバーの登録を消す)。 */
    suspend fun unregister() = registrar.unregister()

    /** 通知のタップ。サインアウト中は開かない。 */
    suspend fun open(appId: String, openApp: suspend (String) -> Unit) {
        if (uid() == null) return
        openApp(appId)
    }

    fun consentAllowed(appId: String): Boolean = uid()?.let { consentStore.allowed(it, appId) } ?: false

    fun currentUid(): String? = uid()

    companion object {
        /** 共有・公開の記録があるアプリか(自分だけのアプリに他の人の行動は無い)。 */
        fun canSetConsent(dsl: AppDSL): Boolean = dsl.entities.any { it.scope == Scope.SHARED || it.scope == Scope.PUBLIC }

        /**
         * 切り替えを出すか(iOS の `canSetNotifyConsent`): 共有・公開の記録があり、作成者でない(作成者には同意なしで届く)。
         * 役割が分からない時(参加者の口が無い・読めない)は出す。
         */
        fun canSetConsent(dsl: AppDSL, member: AppMembersStore.Entry?): Boolean = canSetConsent(dsl) && member?.isOwner != true

        /** 参加者を読み直してから [canSetConsent] を決める(設定を開いた時)。 */
        suspend fun canSetConsent(dsl: AppDSL, appId: String, members: AppMembersStore?): Boolean {
            if (!canSetConsent(dsl)) return false
            members?.reload(appId)
            return canSetConsent(dsl, members?.get(appId))
        }
    }
}

/**
 * 通知のタップで開くアプリを画面へ渡す(押した時は Activity の intent で来る)。
 * 開いた側が [consume] する。同じ appId を続けて押しても届くよう、消してから次を受ける。
 */
object PushOpen {
    private val _pending = MutableStateFlow<String?>(null)
    val pending: StateFlow<String?> = _pending.asStateFlow()

    fun offer(appId: String?) {
        if (appId != null) _pending.value = appId
    }

    fun consume(): String? = _pending.value.also { _pending.value = null }
}
