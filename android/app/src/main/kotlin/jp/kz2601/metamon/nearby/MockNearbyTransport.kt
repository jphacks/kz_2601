package jp.kz2601.metamon.nearby

import jp.kz2601.metamon.dsl.nearby.NearbyShare

/**
 * 端末の中だけの通り道(iOS の `METAMON_MOCK` のモックに当たる)。同じ [Network] の端末どうしが見つけ合い、切れをそのまま渡す。
 * 単体テストは 2 台を 1 つの [Network] に入れて送受信の流れを確かめる。`METAMON_MOCK=1` の時は [withEchoPeer] で
 * 「受け取る」を押す相手を 1 人置き、実機 2 台が無くても送る画面を確かめられる。知らせは呼んだその場で返す(同じスレッド)。
 */
class MockNearbyTransport(private val network: Network, val id: String) : NearbyTransport {
    override var listener: NearbyTransport.Listener? = null
    internal var advertisedName: String? = null
        private set
    private var discovering = false

    /** 端末どうしの場。 */
    class Network {
        internal val devices = mutableListOf<MockNearbyTransport>()
        internal val links = mutableSetOf<Pair<String, String>>()

        fun device(id: String): MockNearbyTransport = MockNearbyTransport(this, id).also { devices += it }

        internal fun find(id: String) = devices.firstOrNull { it.id == id }
        internal fun linked(a: String, b: String) = (a to b) in links || (b to a) in links

        internal fun announce() {
            for (d in devices) if (d.discovering) for (o in devices) if (o !== d) o.advertisedName?.let { d.listener?.onFound(o.id, it) }
        }
    }

    override fun startAdvertising(name: String) {
        advertisedName = name
        network.announce()
    }

    override fun stopAdvertising() {
        if (advertisedName == null) return
        advertisedName = null
        network.devices.filter { it.discovering && it !== this }.forEach { it.listener?.onLost(id) }
    }

    override fun startDiscovery() {
        discovering = true
        network.announce()
    }

    override fun stopDiscovery() {
        discovering = false
    }

    override fun connect(endpointId: String, name: String) {
        val other = network.find(endpointId)
        if (other?.advertisedName == null) {
            listener?.onDisconnected(endpointId)
            return
        }
        network.links += id to endpointId
        other.listener?.onConnected(id, name, outgoing = false)
        listener?.onConnected(endpointId, other.advertisedName.orEmpty(), outgoing = true)
    }

    override fun send(endpointId: String, frame: ByteArray) {
        require(frame.size <= NearbyWire.MAX_CHUNK_BYTES) { "1 切れが大きすぎる" }
        if (!network.linked(id, endpointId)) return
        network.find(endpointId)?.listener?.onFrame(id, frame)
    }

    override fun disconnect(endpointId: String) {
        if (!network.links.remove(id to endpointId) && !network.links.remove(endpointId to id)) return
        network.find(endpointId)?.listener?.onDisconnected(id)
        listener?.onDisconnected(endpointId)
    }

    override fun stopAll() {
        stopAdvertising()
        stopDiscovery()
        network.links.filter { it.first == id || it.second == id }.forEach { disconnect(if (it.first == id) it.second else it.first) }
    }

    companion object {
        const val ECHO_NAME = "Test peer"

        /** 送る画面を 1 台で確かめるための相手(届いた Header に READY、Payload に件数の ACK を返す)。 */
        fun withEchoPeer(): MockNearbyTransport {
            val network = Network()
            val peer = network.device("echo")
            peer.listener = object : NearbyTransport.Listener {
                private val reader = NearbyWire.Reader()
                override fun onFound(endpointId: String, name: String) = Unit
                override fun onLost(endpointId: String) = Unit
                override fun onConnected(endpointId: String, name: String, outgoing: Boolean) = Unit
                override fun onDisconnected(endpointId: String) = Unit
                override fun onError(advertising: Boolean) = Unit
                override fun onFrame(endpointId: String, frame: ByteArray) {
                    val m = reader.accept(frame) ?: return
                    val reply = when (m.kind) {
                        NearbyWire.Kind.HEADER -> NearbyWire.frames(NearbyWire.Kind.READY, ByteArray(0))
                        NearbyWire.Kind.PAYLOAD -> {
                            val added = NearbyShare.decodePayload(m.body)?.records?.size ?: 0
                            NearbyWire.frames(NearbyWire.Kind.ACK, NearbyShare.encode(NearbyShare.Ack(ok = added > 0, added = added)))
                        }
                        else -> return
                    }
                    reply.forEach { peer.send(endpointId, it) }
                }
            }
            peer.startAdvertising(ECHO_NAME)
            return network.device("me")
        }
    }
}
