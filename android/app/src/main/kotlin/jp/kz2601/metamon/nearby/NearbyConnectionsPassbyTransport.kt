package jp.kz2601.metamon.nearby

import android.os.Handler
import android.os.Looper
import android.util.Log
import com.google.android.gms.nearby.connection.AdvertisingOptions
import com.google.android.gms.nearby.connection.ConnectionInfo
import com.google.android.gms.nearby.connection.ConnectionLifecycleCallback
import com.google.android.gms.nearby.connection.ConnectionResolution
import com.google.android.gms.nearby.connection.ConnectionsClient
import com.google.android.gms.nearby.connection.DiscoveredEndpointInfo
import com.google.android.gms.nearby.connection.DiscoveryOptions
import com.google.android.gms.nearby.connection.EndpointDiscoveryCallback
import com.google.android.gms.nearby.connection.Payload
import com.google.android.gms.nearby.connection.PayloadCallback
import com.google.android.gms.nearby.connection.PayloadTransferUpdate
import com.google.android.gms.nearby.connection.Strategy
import jp.kz2601.metamon.dsl.nearby.NearbyLimits
import jp.kz2601.metamon.dsl.nearby.NearbyPassby
import jp.kz2601.metamon.renderer.components.nearby.PassbyDistanceSupport
import jp.kz2601.metamon.renderer.components.nearby.PassbyPeer

/**
 * すれ違い通信の Nearby Connections の通り道(iOS の `MCPassbyTransport`)。近くの同じアプリを探して自動で交換する。
 *
 * - 同じアプリかは endpointName の印([PassbyTag]・appId のハッシュ)で判定。申し出は senderId の小さい側だけ(二重接続を防ぐ)
 * - 相手ごとに 1 本つなぎ、互いに Payload(JSON・BYTES 1 通)を 1 回送る。交換が済んだら切る
 * - 受け取る 1 通は [NearbyPassby.MAX_RECEIVE_BYTES] まで(読む前に大きさを確かめる)。名前・id の長さは [NearbyLimits]
 * - 距離・方向は測らない(UWB の端末が限られる)。distance の部品でも交換したら切る
 * - Strategy は多対多(P2P_CLUSTER)。送るだけの近距離共有(1 対 1)とは serviceId を分ける
 * - 近距離共有と同じ [client] を使うので、止める時は [shared] を通して自分の分だけ止める(相手の送受信を切らない)
 */
class NearbyConnectionsPassbyTransport(
    private val client: ConnectionsClient,
    private val shared: SharedNearbyClient,
    /** 失敗の案内(画面の言語で) */
    private val discoveryFailed: () -> String,
) : PassbyTransport {
    private val main = Handler(Looper.getMainLooper())

    override var onPayload: ((NearbyPassby.Payload) -> Unit)? = null
    override var onPeers: ((List<PassbyPeer>) -> Unit)? = null
    override var onNotice: ((String?) -> Unit)? = null
    override val distanceSupport: PassbyDistanceSupport = PassbyDistanceSupport.Unsupported()

    private class Link(val senderId: String) {
        var name: String = NearbyLimits.ANONYMOUS_NAME
        var connected = false
        var sent = false
        var received = false
    }

    private var current: PassbyStart? = null
    private var tag = ""
    private val links = mutableMapOf<String, Link>()

    private val payloads = object : PayloadCallback() {
        override fun onPayloadReceived(endpointId: String, payload: Payload) {
            val bytes = payload.asBytes() ?: return // BYTES だけ使う(STREAM・FILE は受けない)
            received(endpointId, bytes)
        }

        override fun onPayloadTransferUpdate(endpointId: String, update: PayloadTransferUpdate) = Unit
    }

    private val lifecycle = object : ConnectionLifecycleCallback() {
        override fun onConnectionInitiated(endpointId: String, info: ConnectionInfo) {
            val cur = current
            val hello = PassbyTag.decode(info.endpointName)
            val link = links[endpointId]
            val ok = when {
                cur == null || hello == null -> false
                // 自分から申し出た相手: 見つけた時の印と同じか
                link != null -> !info.isIncomingConnection && link.senderId == hello.senderId
                else -> info.isIncomingConnection && PassbyTag.shouldAccept(cur.hello, hello, cur.shouldExchange)
            }
            if (!ok || hello == null) {
                links.remove(endpointId)
                shared.closed(endpointId)
                client.rejectConnection(endpointId)
                return
            }
            if (link == null) links[endpointId] = Link(hello.senderId)
            shared.opened(SERVICE_ID, endpointId)
            client.acceptConnection(endpointId, payloads)
        }

        override fun onConnectionResult(endpointId: String, resolution: ConnectionResolution) {
            val link = links[endpointId] ?: return
            if (!resolution.status.isSuccess) {
                links.remove(endpointId)
                shared.closed(endpointId)
                publishPeers()
                return
            }
            connected(endpointId, link)
        }

        override fun onDisconnected(endpointId: String) {
            shared.closed(endpointId)
            if (links.remove(endpointId) != null) publishPeers()
        }
    }

    private val discovery = object : EndpointDiscoveryCallback() {
        override fun onEndpointFound(endpointId: String, info: DiscoveredEndpointInfo) {
            val cur = current ?: return
            if (info.serviceId != SERVICE_ID || links.containsKey(endpointId)) return
            val hello = PassbyTag.decode(info.endpointName) ?: return
            if (!PassbyTag.shouldInvite(cur.hello, hello, cur.shouldExchange)) return
            links[endpointId] = Link(hello.senderId)
            shared.opened(SERVICE_ID, endpointId)
            client.requestConnection(tag, endpointId, lifecycle).addOnFailureListener {
                Log.w(TAG, "connect failed: ${it.javaClass.simpleName}")
                links.remove(endpointId)
                shared.closed(endpointId)
            }
        }

        override fun onEndpointLost(endpointId: String) = Unit
    }

    override fun start(start: PassbyStart) {
        stop()
        current = start
        tag = PassbyTag.encode(start.hello)
        onNotice?.invoke(null)
        shared.willAdvertise(SERVICE_ID)
        client.startAdvertising(tag, SERVICE_ID, lifecycle, AdvertisingOptions.Builder().setStrategy(STRATEGY).build())
            .addOnFailureListener {
                shared.advertisingFailed(SERVICE_ID)
                failed("advertise", it)
            }
        shared.willDiscover(SERVICE_ID)
        client.startDiscovery(SERVICE_ID, discovery, DiscoveryOptions.Builder().setStrategy(STRATEGY).build())
            .addOnFailureListener {
                shared.discoveryFailed(SERVICE_ID)
                failed("discovery", it)
            }
    }

    override fun stop() {
        main.removeCallbacksAndMessages(null)
        if (current != null) {
            // stopAdvertising / stopDiscovery / stopAllEndpoints はアプリ全体に効く(近くの人に送るのを切る)ので使わない
            shared.stopAdvertising(SERVICE_ID)
            shared.stopDiscovery(SERVICE_ID)
            shared.disconnectAll(SERVICE_ID)
        }
        current = null
        links.clear()
        publishPeers()
    }

    private fun failed(what: String, e: Exception) {
        Log.w(TAG, "$what failed: ${e.javaClass.simpleName}")
        if (current != null) onNotice?.invoke(discoveryFailed())
    }

    private fun connected(endpointId: String, link: Link) {
        val cur = current ?: return
        link.connected = true
        val bytes = NearbyPassby.encode(cur.payload())
        client.sendPayload(endpointId, Payload.fromBytes(bytes)).addOnFailureListener {
            Log.w(TAG, "send failed: ${it.javaClass.simpleName}")
            shared.disconnect(endpointId)
            if (links.remove(endpointId) != null) publishPeers()
        }
        link.sent = true
        publishPeers()
        finishIfDone(endpointId, link)
    }

    private fun received(endpointId: String, data: ByteArray) {
        val link = links[endpointId] ?: return
        // 大きさ・名前・id の長さを読む前に確かめる(上限を超えたら取り込まない)
        val payload = NearbyPassby.decodePayload(data)
        if (payload != null && !link.received) {
            link.received = true
            link.name = payload.name
            onPayload?.invoke(payload.copy(senderId = link.senderId)) // 申し出の時の id に揃える(なりすまし防止)
        }
        publishPeers()
        finishIfDone(endpointId, link)
    }

    /** 交換が済んだら少し待って切る(相手が受け取り終わる前に切らない)。 */
    private fun finishIfDone(endpointId: String, link: Link) {
        if (!link.sent || !link.received) return
        main.postDelayed({
            if (links[endpointId] !== link) return@postDelayed
            shared.disconnect(endpointId)
            links.remove(endpointId)
            publishPeers()
        }, LINGER_MILLIS)
    }

    private fun publishPeers() {
        onPeers?.invoke(links.values.filter { it.connected }.map { PassbyPeer(it.senderId, it.name) })
    }

    companion object {
        /** すれ違い通信どうしだけが見つけ合う印(近くの人に送るのとは別)。 */
        const val SERVICE_ID = "jp.kz2601.metamon.passby"
        private val STRATEGY = Strategy.P2P_CLUSTER
        private const val LINGER_MILLIS = 1_000L
        private const val TAG = "Passby"
    }
}
