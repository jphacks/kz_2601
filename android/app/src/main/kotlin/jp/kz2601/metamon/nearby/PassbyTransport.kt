package jp.kz2601.metamon.nearby

import jp.kz2601.metamon.dsl.nearby.NearbyLimits
import jp.kz2601.metamon.dsl.nearby.NearbyPassby
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.renderer.components.nearby.PassbyDistanceSupport
import jp.kz2601.metamon.renderer.components.nearby.PassbyPeer

/** すれ違い通信を始める時に渡すもの(iOS の `PassbyStart`)。 */
class PassbyStart(
    val hello: NearbyPassby.Hello,
    val config: NearbyPassby.Config,
    val entity: Entity,
    /** 送るデータ(つながった時に作る = 最新の記録) */
    val payload: () -> NearbyPassby.Payload,
    /** この相手(senderId)とまだ交換してよいか(cooldown) */
    val shouldExchange: (String) -> Boolean,
)

/**
 * すれ違い通信の通り道(iOS の `PassbyTransport`)。実機は [NearbyConnectionsPassbyTransport]、`METAMON_MOCK=1` は
 * [MockPassbyTransport]。呼び出しと知らせはメインスレッド。
 */
interface PassbyTransport {
    var onPayload: ((NearbyPassby.Payload) -> Unit)?
    var onPeers: ((List<PassbyPeer>) -> Unit)?

    /** 直近の失敗・案内(null で消す)。 */
    var onNotice: ((String?) -> Unit)?
    val distanceSupport: PassbyDistanceSupport
    fun start(start: PassbyStart)
    fun stop()
}

/**
 * 近くの端末に見せる名前(Nearby Connections の endpointName)に載せる印。iOS の discoveryInfo(`v`・`a`・`s`)に当たる。
 * 名前は載せない(つないだ後の Payload で渡す)ので、匿名を切り替えても印は変わらない。
 */
object PassbyTag {
    private const val SEP = '|'

    fun encode(hello: NearbyPassby.Hello): String = "${hello.v}$SEP${hello.app}$SEP${hello.senderId}"

    /** 読めない・上限を超える・id が不正なものは null。 */
    fun decode(tag: String?): NearbyPassby.Hello? {
        if (tag == null || tag.toByteArray(Charsets.UTF_8).size > NearbyPassby.MAX_HELLO_BYTES) return null
        val parts = tag.split(SEP)
        if (parts.size != 3) return null
        val v = parts[0].toIntOrNull() ?: return null
        return NearbyPassby.Hello(v, parts[1], parts[2], name = "").validated()?.copy(name = NearbyLimits.ANONYMOUS_NAME)
    }

    /** 見つけた相手に申し出るか: 同じアプリ・自分でない・senderId の小さい側だけ(二重接続を防ぐ)・cooldown の外。 */
    fun shouldInvite(me: NearbyPassby.Hello, other: NearbyPassby.Hello, shouldExchange: (String) -> Boolean): Boolean =
        other.app == me.app && other.senderId != me.senderId && me.senderId < other.senderId && shouldExchange(other.senderId)

    /** 届いた申し出を受けるか: 同じアプリ・自分でない・cooldown の外。 */
    fun shouldAccept(me: NearbyPassby.Hello, other: NearbyPassby.Hello, shouldExchange: (String) -> Boolean): Boolean =
        other.app == me.app && other.senderId != me.senderId && shouldExchange(other.senderId)
}
