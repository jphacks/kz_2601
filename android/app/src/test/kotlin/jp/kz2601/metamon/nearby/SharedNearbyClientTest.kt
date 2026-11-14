package jp.kz2601.metamon.nearby

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * 近くの人に送る(SHARE)とすれ違い通信(PASSBY)は 1 つの ConnectionsClient を分け合う。
 * 片方を止めても、もう片方の待ち受け・探す・つながりは切らない(止めるのは最後の 1 人・自分のつながりだけ)。
 */
class SharedNearbyClientTest {
    private class FakeRadio : SharedNearbyClient.Radio {
        val calls = mutableListOf<String>()
        override fun stopAdvertising() {
            calls += "stopAdvertising"
        }

        override fun stopDiscovery() {
            calls += "stopDiscovery"
        }

        override fun disconnect(endpointId: String) {
            calls += "disconnect:$endpointId"
        }
    }

    private val radio = FakeRadio()
    private val shared = SharedNearbyClient(radio)

    /** すれ違い通信の通り道の start / stop と同じ順で呼ぶ。 */
    private fun passbyStart() {
        shared.willAdvertise(PASSBY)
        shared.willDiscover(PASSBY)
    }

    private fun passbyStop() {
        shared.stopAdvertising(PASSBY)
        shared.stopDiscovery(PASSBY)
        shared.disconnectAll(PASSBY)
    }

    /** 近くの人に送るの stopAll と同じ順で呼ぶ。 */
    private fun shareStopAll() {
        shared.stopAdvertising(SHARE)
        shared.stopDiscovery(SHARE)
        shared.disconnectAll(SHARE)
    }

    @Test
    fun stoppingPassbyKeepsAnActiveShareSession() {
        shared.willAdvertise(SHARE)
        shared.willDiscover(SHARE)
        shared.opened(SHARE, "share-peer")
        passbyStart()
        shared.opened(PASSBY, "passby-peer")

        passbyStop()

        // 送受信中の相手・待ち受け・探すはそのまま。すれ違いの相手だけ切る
        assertEquals(listOf("disconnect:passby-peer"), radio.calls)
    }

    @Test
    fun stoppingShareKeepsPassby() {
        passbyStart()
        shared.opened(PASSBY, "passby-peer")
        shared.willAdvertise(SHARE)
        shared.opened(SHARE, "share-peer")

        shareStopAll()

        assertEquals(listOf("disconnect:share-peer"), radio.calls)
    }

    @Test
    fun lastUserStopsAdvertisingAndDiscovery() {
        shared.willAdvertise(SHARE)
        passbyStart()
        // 探していたのはすれ違いだけなので探すのは止まり、待ち受けは近くの人に送るのが残る
        passbyStop()
        assertEquals(listOf("stopDiscovery"), radio.calls)
        shareStopAll()
        assertEquals(listOf("stopDiscovery", "stopAdvertising"), radio.calls)
    }

    @Test
    fun aloneEachFeatureStopsWhatItStarted() {
        passbyStart()
        shared.opened(PASSBY, "a")
        shared.opened(PASSBY, "b")
        passbyStop()
        assertEquals(listOf("stopAdvertising", "stopDiscovery", "disconnect:a", "disconnect:b"), radio.calls)
    }

    @Test
    fun failedStartDoesNotHoldTheRadioOrStopTheOther() {
        // 近くの人に送るが待ち受け中に、すれ違いの待ち受けが始められなかった(Nearby は同時に 1 つだけ)
        shared.willAdvertise(SHARE)
        shared.willAdvertise(PASSBY)
        shared.advertisingFailed(PASSBY)
        passbyStop()
        assertTrue(radio.calls.isEmpty())
        // 近くの人に送るが止めれば実際に止まる(すれ違いが残っていない)
        shareStopAll()
        assertEquals(listOf("stopAdvertising"), radio.calls)
    }

    @Test
    fun stoppingWhatWasNeverStartedDoesNothing() {
        shared.willAdvertise(SHARE)
        shared.willDiscover(SHARE)
        passbyStop()
        passbyStop()
        assertTrue(radio.calls.isEmpty())
    }

    @Test
    fun closedEndpointsAreNotDisconnectedAgain() {
        shared.opened(SHARE, "x")
        shared.opened(SHARE, "y")
        shared.closed("x") // 相手から切れた
        shared.disconnect("y") // 自分から切った
        shareStopAll()
        assertEquals(listOf("disconnect:y"), radio.calls)
    }

    private companion object {
        const val SHARE = NearbyConnectionsTransport.SERVICE_ID
        const val PASSBY = NearbyConnectionsPassbyTransport.SERVICE_ID
    }
}
