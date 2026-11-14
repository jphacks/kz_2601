package jp.kz2601.metamon.nearby

/**
 * 近くの端末との通り道。本物は [NearbyConnectionsTransport](Nearby Connections)、テスト・モックは [MockNearbyTransport]。
 * 呼び出しと知らせ([Listener])はメインスレッド。つなぐ申し出は通り道の中で受けてよい(取り込むかは [NearbyService] が決める)。
 */
interface NearbyTransport {
    var listener: Listener?

    /** 受ける側: 近くの端末から見えるようにする。[name] は相手の一覧に出る名前。 */
    fun startAdvertising(name: String)
    fun stopAdvertising()

    /** 送る側: 近くの端末を探す。 */
    fun startDiscovery()
    fun stopDiscovery()

    /** 送る側: [endpointId] につなぐ。つながったら [Listener.onConnected]、だめなら [Listener.onDisconnected]。 */
    fun connect(endpointId: String, name: String)

    /** 1 切れ([NearbyWire.MAX_CHUNK_BYTES] 以内)を送る。 */
    fun send(endpointId: String, frame: ByteArray)
    fun disconnect(endpointId: String)

    /** 全部やめる(待ち受け・探す・つながり)。 */
    fun stopAll()

    interface Listener {
        fun onFound(endpointId: String, name: String)
        fun onLost(endpointId: String)

        /** つながった。[name] は相手の名前、[outgoing] は自分から申し出たか。 */
        fun onConnected(endpointId: String, name: String, outgoing: Boolean)
        fun onDisconnected(endpointId: String)
        fun onFrame(endpointId: String, frame: ByteArray)

        /** 待ち受け・探すのを始められなかった(権限・Bluetooth がオフなど)。 */
        fun onError(advertising: Boolean)
    }
}
