package jp.kz2601.metamon.nearby

import jp.kz2601.metamon.R
import jp.kz2601.metamon.dsl.nearby.NearbyLimits
import jp.kz2601.metamon.dsl.nearby.NearbyShare
import jp.kz2601.metamon.dsl.nearby.NearbyShare.FieldSignature
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.nearby.NearbyService.SendState
import jp.kz2601.metamon.runtime.UiText
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.runCurrent
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** 近くの人に送る(#1382)の送受信の流れ。端末の中だけの通り道(MockNearbyTransport)で 2 台を動かす。 */
@OptIn(ExperimentalCoroutinesApi::class)
class NearbyServiceTest {
    private val scope = TestScope()
    private val network = MockNearbyTransport.Network()
    private val sender = NearbyService(network.device("A"), scope, "花子")
    private val receiver = NearbyService(network.device("B"), scope, "太郎")
    private val received = mutableListOf<NearbyShare.Payload>()

    init {
        receiver.decide = { NearbyShare.Decision.Accept(it.entityId, sameApp = true) }
        receiver.onReceive = { p, _ ->
            received += p
            NearbyShare.Ack(ok = true, added = p.records.size)
        }
    }

    private fun payload(rows: Int, text: String = "a", from: String = "花子"): NearbyShare.Payload {
        val header = NearbyShare.Header(
            sender = from, appId = "app1", appName = "出席カード", entityId = "attendance", entityName = "出席",
            fields = listOf(FieldSignature("name", "string")), count = rows,
        )
        return NearbyShare.Payload(header, List(rows) { mapOf("name" to JSONValue.Str(text)) })
    }

    private fun openSheet(): NearbyService.Peer {
        receiver.setAdvertising(true)
        sender.startBrowsing()
        return sender.peers.value.single()
    }

    @Test fun findsPeerByName() {
        val peer = openSheet()
        assertEquals(NearbyService.Peer("B", "太郎"), peer)
        receiver.setAdvertising(false)
        assertTrue("待ち受けをやめたら一覧から消える", sender.peers.value.isEmpty())
    }

    @Test fun acceptDeliversAndAcks() {
        val peer = openSheet()
        sender.send(payload(3), peer)
        assertEquals(SendState.Inviting("太郎"), sender.sendState.value)
        val incoming = receiver.incoming.value!!
        assertEquals("花子", incoming.header.sender)
        assertEquals(3, incoming.header.count)
        assertTrue("承認するまで取り込まない", received.isEmpty())

        receiver.answer(incoming, accept = true)
        scope.runCurrent()
        assertEquals(1, received.size)
        assertEquals(3, received.single().records.size)
        assertEquals(SendState.Done(UiText.Plural(R.plurals.nearby_delivered, 3, listOf("太郎", 3))), sender.sendState.value)
        assertTrue("受け渡しが終わったら切る", network.links.isEmpty())
    }

    @Test fun declineFailsTheSender() {
        val peer = openSheet()
        sender.send(payload(1), peer)
        receiver.answer(receiver.incoming.value!!, accept = false)
        scope.runCurrent()
        assertTrue(received.isEmpty())
        assertEquals(SendState.Failed(UiText.Res(R.string.nearby_declined, "太郎")), sender.sendState.value)
        assertNull(receiver.incoming.value)
    }

    @Test fun rejectDecisionCannotBeAccepted() {
        receiver.decide = { NearbyShare.Decision.Reject("このアプリには「出席」がないため受け取れません") }
        val peer = openSheet()
        sender.send(payload(1), peer)
        val item = receiver.incoming.value!!
        assertTrue(item.decision is NearbyShare.Decision.Reject)
        receiver.answer(item, accept = true)
        scope.runCurrent()
        assertTrue("取り込めない時は承認しても受け取らない", received.isEmpty())
        assertTrue(sender.sendState.value is SendState.Failed)
    }

    @Test fun largePayloadIsSplitAndJoined() {
        val peer = openSheet()
        val big = payload(NearbyShare.MAX_SEND_RECORDS, text = "あ".repeat(3000)) // 1 件 約 9 KB・全体は 32 KB を大きく超える
        assertTrue(NearbyShare.encode(big).size > NearbyWire.MAX_CHUNK_BYTES * 4)
        sender.send(big, peer)
        receiver.answer(receiver.incoming.value!!, accept = true)
        scope.runCurrent()
        assertEquals(big.records, received.single().records)
    }

    @Test fun noAnswerTimesOut() {
        val peer = openSheet()
        sender.send(payload(1), peer)
        scope.advanceTimeBy(NearbyService.TIMEOUT_MILLIS + 1)
        scope.runCurrent()
        assertEquals(SendState.Failed(UiText.Res(R.string.nearby_no_response, "太郎")), sender.sendState.value)
        assertNull("切れたら確認も消える", receiver.incoming.value)
    }

    @Test fun notAdvertisingRefusesConnection() {
        val peer = openSheet()
        receiver.setAdvertising(false)
        sender.send(payload(1), peer)
        assertNull(receiver.incoming.value)
        assertTrue(sender.sendState.value is SendState.Failed)
    }

    @Test fun anonymousNameIsSentAndShownInTheReceiversLanguage() {
        sender.rename(NearbyLimits.peerName("花子", anonymous = true))
        assertEquals(NearbyLimits.ANONYMOUS_NAME, sender.myName.value)
        val peer = openSheet()
        sender.send(payload(1, from = "花子"), peer)
        assertEquals("送る直前に今の名前へ揃える", NearbyLimits.ANONYMOUS_NAME, receiver.incoming.value?.header?.sender)
        assertEquals(UiText.Res(R.string.nearby_anonymous), NearbyService.label(NearbyLimits.ANONYMOUS_NAME))
        assertEquals("太郎", NearbyService.label("太郎"))
    }

    @Test fun renameWhileAdvertisingRestartsWithTheNewName() {
        receiver.setAdvertising(true)
        receiver.rename(NearbyLimits.ANONYMOUS_NAME)
        sender.startBrowsing()
        assertEquals(NearbyLimits.ANONYMOUS_NAME, sender.peers.value.single().name)
    }

    @Test fun headerOverTheLimitIsDropped() {
        val a = network.device("X")
        val b = network.device("Y")
        val service = NearbyService(b, scope, "受け手")
        service.decide = { NearbyShare.Decision.Accept(it.entityId, sameApp = true) }
        service.setAdvertising(true)
        a.connect("Y", "相手")
        val tooBig = ByteArray(NearbyShare.MAX_HEADER_BYTES + 1) { 0x20 }
        NearbyWire.frames(NearbyWire.Kind.HEADER, tooBig).forEach { a.send("Y", it) }
        assertNull(service.incoming.value)
        assertTrue("上限を超えたら切る", network.links.none { it.first == "X" })
    }

    @Test fun payloadWithoutApprovalIsIgnored() {
        val a = network.device("X")
        val b = network.device("Y")
        val service = NearbyService(b, scope, "受け手")
        var called = false
        service.onReceive = { _, _ -> called = true; NearbyShare.Ack(true, 1) }
        service.setAdvertising(true)
        a.connect("Y", "相手")
        NearbyWire.frames(NearbyWire.Kind.PAYLOAD, NearbyShare.encode(payload(1))).forEach { a.send("Y", it) }
        scope.runCurrent()
        assertTrue("承認していない相手の Payload は取り込まない", !called)
    }

    @Test fun echoPeerForMockBuilds() {
        val service = NearbyService(MockNearbyTransport.withEchoPeer(), scope, "わたし")
        service.startBrowsing()
        val peer = service.peers.value.single()
        assertEquals(MockNearbyTransport.ECHO_NAME, peer.name)
        service.send(payload(2), peer)
        assertEquals(SendState.Done(UiText.Plural(R.plurals.nearby_delivered, 2, listOf(MockNearbyTransport.ECHO_NAME, 2))), service.sendState.value)
    }
}
