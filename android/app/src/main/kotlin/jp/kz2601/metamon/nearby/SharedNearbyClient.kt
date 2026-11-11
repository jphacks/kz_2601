package jp.kz2601.metamon.nearby

import com.google.android.gms.nearby.connection.ConnectionsClient

/**
 * 近くの人に送る(#1382)とすれ違い通信(#1383)が分け合う Nearby Connections の止め方の持ち主。
 * ConnectionsClient の stopAdvertising / stopDiscovery / stopAllEndpoints はアプリ全体に効くので、
 * 片方が止めるともう片方の待ち受け・探す・つながりまで切れてしまう。ここで誰が使っているかを数え、
 * 待ち受け・探すは最後の 1 人が止めた時だけ止め、つながりは自分のものだけ切る。呼ぶのはメインスレッドだけ。
 */
class SharedNearbyClient(private val radio: Radio) {
    /** 実際に止める所(本物は [ConnectionsClient])。 */
    interface Radio {
        fun stopAdvertising()
        fun stopDiscovery()
        fun disconnect(endpointId: String)
    }

    private val advertisers = mutableSetOf<String>()
    private val discoverers = mutableSetOf<String>()
    private val endpoints = mutableMapOf<String, String>()

    /** [user] が待ち受けを始める(start を呼ぶ前)。失敗したら [advertisingFailed]。 */
    fun willAdvertise(user: String) {
        advertisers += user
    }

    /** 始められなかった(他の人が待ち受け中など)。止めはしない(他の人の待ち受けを切らない)。 */
    fun advertisingFailed(user: String) {
        advertisers -= user
    }

    /** [user] の待ち受けをやめる。他に待ち受けている人がいれば実際には止めない。 */
    fun stopAdvertising(user: String) {
        if (advertisers.remove(user) && advertisers.isEmpty()) radio.stopAdvertising()
    }

    fun willDiscover(user: String) {
        discoverers += user
    }

    fun discoveryFailed(user: String) {
        discoverers -= user
    }

    fun stopDiscovery(user: String) {
        if (discoverers.remove(user) && discoverers.isEmpty()) radio.stopDiscovery()
    }

    /** [endpointId] は [user] のつながり(申し出た・受けた時)。 */
    fun opened(user: String, endpointId: String) {
        endpoints[endpointId] = user
    }

    /** つながりが切れた・だめだった(こちらから切る時は [disconnect])。 */
    fun closed(endpointId: String) {
        endpoints -= endpointId
    }

    fun disconnect(endpointId: String) {
        endpoints -= endpointId
        radio.disconnect(endpointId)
    }

    /** [user] のつながりだけ全部切る(stopAllEndpoints の代わり)。 */
    fun disconnectAll(user: String) {
        endpoints.filterValues { it == user }.keys.toList().forEach(::disconnect)
    }

    companion object {
        fun of(client: ConnectionsClient) = SharedNearbyClient(object : Radio {
            override fun stopAdvertising() = client.stopAdvertising()
            override fun stopDiscovery() = client.stopDiscovery()
            override fun disconnect(endpointId: String) = client.disconnectFromEndpoint(endpointId)
        })
    }
}
