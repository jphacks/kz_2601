package jp.kz2601.metamon.nearby

import android.content.Context
import android.util.Log
import com.google.android.gms.nearby.Nearby
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

/**
 * Nearby Connections(Bluetooth・Wi-Fi を OS が選ぶ・インターネット不要)の通り道。iOS の MultipeerConnectivity に当たる。
 * 同じ OS どうしだけつながる(iOS の端末とはつながらない・#1384)。Strategy は 1 対 1(P2P_POINT_TO_POINT)。
 * 権限(AppPermission.NEARBY)は呼ぶ前に許可を取っておく。知らせは Play 開発者サービスがメインスレッドで呼ぶ。
 */
class NearbyConnectionsTransport(context: Context) : NearbyTransport {
    private val client: ConnectionsClient = Nearby.getConnectionsClient(context.applicationContext)
    override var listener: NearbyTransport.Listener? = null

    /** 申し出の途中(名前と自分から申し出たか)。つながったら [NearbyTransport.Listener.onConnected] に渡す。 */
    private val pending = mutableMapOf<String, Pair<String, Boolean>>()
    private val outgoing = mutableSetOf<String>()
    private var advertising = false
    private var discovering = false

    private val payloads = object : PayloadCallback() {
        override fun onPayloadReceived(endpointId: String, payload: Payload) {
            val bytes = payload.asBytes() ?: return // BYTES だけ使う(STREAM・FILE は受けない)
            listener?.onFrame(endpointId, bytes)
        }

        override fun onPayloadTransferUpdate(endpointId: String, update: PayloadTransferUpdate) = Unit
    }

    private val lifecycle = object : ConnectionLifecycleCallback() {
        override fun onConnectionInitiated(endpointId: String, info: ConnectionInfo) {
            // 通り道としては受ける。取り込むかは届いた Header を見て NearbyService が利用者に聞く
            pending[endpointId] = info.endpointName to (endpointId in outgoing)
            client.acceptConnection(endpointId, payloads)
        }

        override fun onConnectionResult(endpointId: String, resolution: ConnectionResolution) {
            val (name, mine) = pending.remove(endpointId) ?: ("" to false)
            outgoing.remove(endpointId)
            if (resolution.status.isSuccess) listener?.onConnected(endpointId, name, mine) else listener?.onDisconnected(endpointId)
        }

        override fun onDisconnected(endpointId: String) {
            pending.remove(endpointId)
            outgoing.remove(endpointId)
            listener?.onDisconnected(endpointId)
        }
    }

    private val discovery = object : EndpointDiscoveryCallback() {
        override fun onEndpointFound(endpointId: String, info: DiscoveredEndpointInfo) {
            if (info.serviceId == SERVICE_ID) listener?.onFound(endpointId, info.endpointName)
        }

        override fun onEndpointLost(endpointId: String) {
            listener?.onLost(endpointId)
        }
    }

    override fun startAdvertising(name: String) {
        if (advertising) return
        advertising = true
        client.startAdvertising(name, SERVICE_ID, lifecycle, AdvertisingOptions.Builder().setStrategy(STRATEGY).build())
            .addOnFailureListener {
                Log.w(TAG, "advertise failed: ${it.javaClass.simpleName}")
                advertising = false
                listener?.onError(advertising = true)
            }
    }

    override fun stopAdvertising() {
        if (!advertising) return
        advertising = false
        client.stopAdvertising()
    }

    override fun startDiscovery() {
        if (discovering) return
        discovering = true
        client.startDiscovery(SERVICE_ID, discovery, DiscoveryOptions.Builder().setStrategy(STRATEGY).build())
            .addOnFailureListener {
                Log.w(TAG, "discovery failed: ${it.javaClass.simpleName}")
                discovering = false
                listener?.onError(advertising = false)
            }
    }

    override fun stopDiscovery() {
        if (!discovering) return
        discovering = false
        client.stopDiscovery()
    }

    override fun connect(endpointId: String, name: String) {
        outgoing += endpointId
        client.requestConnection(name, endpointId, lifecycle).addOnFailureListener {
            Log.w(TAG, "connect failed: ${it.javaClass.simpleName}")
            outgoing.remove(endpointId)
            listener?.onDisconnected(endpointId)
        }
    }

    override fun send(endpointId: String, frame: ByteArray) {
        client.sendPayload(endpointId, Payload.fromBytes(frame)).addOnFailureListener {
            Log.w(TAG, "send failed: ${it.javaClass.simpleName}")
            client.disconnectFromEndpoint(endpointId)
            listener?.onDisconnected(endpointId)
        }
    }

    override fun disconnect(endpointId: String) {
        client.disconnectFromEndpoint(endpointId)
    }

    override fun stopAll() {
        stopAdvertising()
        stopDiscovery()
        pending.clear()
        outgoing.clear()
        client.stopAllEndpoints()
    }

    companion object {
        /** 同じアプリ(Tailor It)どうしだけが見つけ合う印。 */
        const val SERVICE_ID = "jp.kz2601.metamon.nearby"
        private val STRATEGY = Strategy.P2P_POINT_TO_POINT
        private const val TAG = "Nearby"
    }
}
