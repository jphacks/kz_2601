package jp.kz2601.metamon.renderer.components.nearby

import jp.kz2601.metamon.dsl.nearby.NearbyLimits
import jp.kz2601.metamon.dsl.nearby.NearbyPassby
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update

/** つながっている相手(距離・方向は測れた時だけ。Android は測らない)。iOS の `PassbyPeer`。 */
data class PassbyPeer(val id: String, val name: String, val distance: Float? = null, val azimuth: Float? = null)

/** 距離・方向を測れるか。[Unsupported.reason] が null なら決まった文言(「この端末は距離・方向の測定に対応していません」)。 */
sealed interface PassbyDistanceSupport {
    data object Supported : PassbyDistanceSupport
    data class Unsupported(val reason: String? = null) : PassbyDistanceSupport
}

/** すれ違い通信の画面に出す状態(iOS の `NearbyPassbyController` の観測できる値)。 */
data class PassbyState(
    /** 同意した同意文の版(null = 未同意) */
    val consentedVersion: Int? = null,
    /** いま探している */
    val isAdvertising: Boolean = false,
    /** 会った人(今のアプリ・新しい順) */
    val encounters: List<NearbyPassby.Encounter> = emptyList(),
    val peers: List<PassbyPeer> = emptyList(),
    val distanceSupport: PassbyDistanceSupport = PassbyDistanceSupport.Unsupported(),
    /** 直近の失敗・案内(近くのデバイスの許可が無い等) */
    val notice: String? = null,
    /** 自分の表示名(名前を出す時に相手に渡る) */
    val displayName: String = "",
    /** 名前を出さない(相手には「近くの人」と渡る)。近くの人に送るのと同じ設定 */
    val isAnonymous: Boolean = false,
    /** オンにした部品の id(今のアプリ) */
    val optedIn: Set<String> = emptySet(),
) {
    /** 相手に見える名前。 */
    val publicName: String get() = NearbyLimits.publicName(displayName, isAnonymous)

    val hasConsent: Boolean get() = (consentedVersion ?: 0) >= NearbyPassby.CONSENT_VERSION
}

/**
 * すれ違い通信(#1383・iOS の `NearbyPassbyController`)の通信側。描画器は Nearby Connections を知らず、アプリ本体が実装を
 * [NearbyHost.passby] で差し込む。呼ぶのはメインスレッドだけ。
 */
interface NearbyPassbyController {
    val state: StateFlow<PassbyState>

    fun isOptedIn(config: NearbyPassby.Config): Boolean
    fun setOptIn(on: Boolean, config: NearbyPassby.Config)

    /** 同意する。[anonymous] = 表示名の代わりに「近くの人」を送る */
    fun agree(anonymous: Boolean)
    fun setAnonymous(on: Boolean)

    /** 画面に出て前面にいる間: オンなら探し始める */
    fun attach(config: NearbyPassby.Config)

    /** 画面を離れた・裏に回った: 止める */
    fun detach(config: NearbyPassby.Config)
}

/** 何も通信しない実装(プレビュー・テスト・ホストが無い時)。iOS の `StaticNearbyPassbyController`。 */
class StaticNearbyPassbyController(initial: PassbyState = PassbyState(displayName = "Tailor It")) : NearbyPassbyController {
    private val _state = MutableStateFlow(initial)
    override val state: StateFlow<PassbyState> = _state.asStateFlow()

    var attached: String? = null
        private set

    fun set(transform: (PassbyState) -> PassbyState) = _state.update(transform)

    override fun isOptedIn(config: NearbyPassby.Config): Boolean = config.componentId in _state.value.optedIn

    override fun setOptIn(on: Boolean, config: NearbyPassby.Config) = _state.update {
        val ids = if (on) it.optedIn + config.componentId else it.optedIn - config.componentId
        it.copy(optedIn = ids, isAdvertising = on && attached == config.componentId)
    }

    override fun agree(anonymous: Boolean) = _state.update { it.copy(consentedVersion = NearbyPassby.CONSENT_VERSION, isAnonymous = anonymous) }

    override fun setAnonymous(on: Boolean) = _state.update { it.copy(isAnonymous = on) }

    override fun attach(config: NearbyPassby.Config) {
        attached = config.componentId
        _state.update { it.copy(isAdvertising = isOptedIn(config)) }
    }

    override fun detach(config: NearbyPassby.Config) {
        if (attached != config.componentId) return
        attached = null
        _state.update { it.copy(isAdvertising = false) }
    }
}
