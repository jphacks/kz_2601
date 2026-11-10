package jp.kz2601.metamon.update

import android.content.Context
import jp.kz2601.metamon.BuildConfig
import jp.kz2601.metamon.services.consent.KeyValueStore
import jp.kz2601.metamon.services.consent.SharedPreferencesKeyValueStore
import jp.kz2601.metamon.services.http.ClientInfo
import jp.kz2601.metamon.services.http.DeviceContext
import jp.kz2601.metamon.services.http.MetamonHttp
import jp.kz2601.metamon.services.http.RetryPolicy
import jp.kz2601.metamon.services.http.TokenProvider
import jp.kz2601.metamon.services.update.HttpClientConfigClient
import jp.kz2601.metamon.services.update.NoticeDismissals
import jp.kz2601.metamon.services.update.ServiceNotice
import jp.kz2601.metamon.services.update.UpdateChecker
import jp.kz2601.metamon.services.update.UpdateRequirement
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.sync.Mutex
import okhttp3.OkHttpClient

/**
 * 最低対応版とアプリ内のお知らせ(#302・#333・iOS の `UpdateGate`)。起動時と前面に戻った時に `GET /v1/client-config` を読む。
 * - min 未満: 閉じられない「アップデートしてください」画面
 * - latest 未満: 閉じられるお知らせ(同じ latest には 1 起動で 1 回)
 * - 通信失敗・設定なし・モック: 何もしない(fail open)
 * 今の backend は Android の版を返さないので、版の判定は値が届くまで何もしない(#906)。お知らせは出る。
 */
class UpdateGate(private val checker: UpdateChecker?, private val dismissals: NoticeDismissals) {
    data class State(
        val requirement: UpdateRequirement = UpdateRequirement.None,
        val storeUrl: String? = null,
        val notice: ServiceNotice? = null,
        val dismissedLatest: Int? = null,
        /** 閉じた id(画面に変化を伝える。端末への記録は [NoticeDismissals]) */
        val dismissedNoticeId: String? = null,
    ) {
        val isBlocking: Boolean get() = requirement is UpdateRequirement.Required

        val showsRecommended: Boolean
            get() = !isBlocking && (requirement as? UpdateRequirement.Recommended)?.let { it.latest != dismissedLatest } == true
    }

    private val _state = MutableStateFlow(State())
    val state: StateFlow<State> = _state.asStateFlow()
    private val checking = Mutex()

    /** バナーに出すお知らせ。閉じられない更新画面の間は出さない。 */
    fun visibleNotice(s: State = state.value): ServiceNotice? {
        val notice = s.notice ?: return null
        if (s.isBlocking || notice.id == s.dismissedNoticeId || dismissals.isDismissed(notice.id)) return null
        return notice
    }

    /** 重ねて呼ばれたら後の方は何もしない(iOS の `checking`)。 */
    suspend fun check() {
        val checker = checker ?: return
        if (!checking.tryLock()) return
        try {
            val result = checker.check()
            _state.update { it.copy(requirement = result.requirement, storeUrl = result.storeUrl, notice = result.notice) }
        } finally {
            checking.unlock()
        }
    }

    /** 閉じた id を端末に覚える(同じ id は二度と出さない)。 */
    fun dismissNotice() {
        val notice = state.value.notice ?: return
        dismissals.dismiss(notice.id)
        _state.update { it.copy(dismissedNoticeId = notice.id) }
    }

    fun dismissRecommended() {
        val latest = (state.value.requirement as? UpdateRequirement.Recommended)?.latest ?: return
        _state.update { it.copy(dismissedLatest = latest) }
    }

    companion object {
        /** 何もしない(モック・テスト)。iOS もモックでは client-config を読まない。 */
        fun disabled(store: KeyValueStore): UpdateGate = UpdateGate(null, NoticeDismissals(store))

        /** appconfig を読む本物。トークンも App Check も付けない(サインイン前にも読む)。 */
        fun live(context: Context, appConfigBaseUrl: String, client: OkHttpClient, device: DeviceContext): UpdateGate {
            val build = BuildConfig.VERSION_CODE.toString()
            val http = MetamonHttp(
                appConfigBaseUrl,
                tokens = TokenProvider { null },
                clientInfo = ClientInfo(version = BuildConfig.VERSION_NAME, build = build),
                client = client,
                // 失敗は何もしない(fail open)ので、やり直さず次の前面で読み直す
                retry = RetryPolicy(idempotentAttempts = 1),
                device = device,
            )
            return UpdateGate(UpdateChecker(HttpClientConfigClient(http), build), NoticeDismissals(SharedPreferencesKeyValueStore.of(context, PREFS)))
        }

        private const val PREFS = "metamon-notices"
    }
}
