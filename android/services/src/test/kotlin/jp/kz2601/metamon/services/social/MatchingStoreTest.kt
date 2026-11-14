package jp.kz2601.metamon.services.social

import jp.kz2601.metamon.dsl.records.InterestState
import jp.kz2601.metamon.dsl.records.MatchBox
import jp.kz2601.metamon.dsl.records.MatchInterest
import jp.kz2601.metamon.services.consent.InMemoryKeyValueStore
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.async
import kotlinx.coroutines.test.UnconfinedTestDispatcher
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Test
import java.io.IOException

/** 仲間探しの一覧(iOS の MatchingStoreTests と同じ決まり・#877): 状態の移り変わり・楽観的な操作・失敗で戻す。 */
@OptIn(ExperimentalCoroutinesApi::class)
class MatchingStoreTest {
    private class FakeClient : MatchingClient {
        val lists = mutableMapOf<MatchBox, List<MatchInterest>>()
        var fail: Exception? = null
        var gate: CompletableDeferred<Unit>? = null
        var sendResult: (String) -> MatchInterest = { MatchInterest(it, status = MatchInterest.PENDING) }
        val calls = mutableListOf<String>()

        private suspend fun step(name: String) {
            calls += name
            gate?.await()
            fail?.let { throw it }
        }

        override suspend fun sendInterest(appId: String, recordId: String, fromRecordId: String, ageConfirmed: Boolean): MatchInterest {
            step("send $recordId from $fromRecordId age=$ageConfirmed")
            return sendResult(recordId)
        }

        override suspend fun list(appId: String, box: MatchBox): List<MatchInterest> {
            step("list ${box.wire}")
            return lists[box].orEmpty()
        }

        override suspend fun accept(appId: String, uid: String, ageConfirmed: Boolean): MatchInterest {
            step("accept $uid")
            return MatchInterest(uid, status = MatchInterest.MATCHED)
        }

        override suspend fun decline(appId: String, uid: String) = step("decline $uid")

        override suspend fun unmatch(appId: String, uid: String) = step("unmatch $uid")
    }

    private val client = FakeClient()
    private val store = MatchingStore("app-1", client)

    @Test
    fun stateComesFromTheLoadedBoxes() = runTest {
        client.lists[MatchBox.INCOMING] = listOf(MatchInterest("a", "pa"))
        client.lists[MatchBox.OUTGOING] = listOf(MatchInterest("b", "pb"))
        client.lists[MatchBox.MATCHES] = listOf(MatchInterest("c", "pc", status = "matched"))
        assertNull(store.items(MatchBox.INCOMING)) // まだ読んでいない
        MatchBox.entries.forEach { store.load(it) }
        assertEquals(InterestState.INCOMING, store.state("a"))
        assertEquals(InterestState.SENT, store.state("b"))
        assertEquals(InterestState.MATCHED, store.state("c"))
        assertEquals(InterestState.NONE, store.state("z"))
    }

    @Test
    fun sendIsOptimisticThenSettledByTheServer() = runTest(UnconfinedTestDispatcher()) {
        client.gate = CompletableDeferred()
        val job = async { store.send("b", recordId = "pb", fromRecordId = "mine") }
        // 返事の前に「送りました」・送信中
        assertEquals(InterestState.SENT, store.state("b"))
        assertTrue(store.isBusy("b"))
        client.gate!!.complete(Unit)
        job.await()
        assertFalse(store.isBusy("b"))
        assertEquals(listOf("send pb from mine age=true"), client.calls) // 年齢は送らず、確認したことだけ
    }

    @Test
    fun sendToSomeoneWhoAlreadyAskedConnectsAtOnce() = runTest {
        client.lists[MatchBox.INCOMING] = listOf(MatchInterest("a", "pa"))
        store.load(MatchBox.INCOMING)
        client.sendResult = { MatchInterest("a", it, status = MatchInterest.MATCHED) }
        val result = store.send("a", "pa", "mine")
        assertTrue(result.isMatched)
        assertEquals(InterestState.MATCHED, store.state("a"))
        assertEquals(emptyList<MatchInterest>(), store.items(MatchBox.INCOMING))
    }

    @Test
    fun failedSendRollsBack() = runTest {
        client.fail = IOException("offline")
        try {
            store.send("b", "pb", "mine")
            fail("throw するはず")
        } catch (_: IOException) {
        }
        assertEquals(InterestState.NONE, store.state("b"))
        assertFalse(store.isBusy("b"))
    }

    @Test
    fun acceptMovesIncomingToMatches() = runTest {
        client.lists[MatchBox.INCOMING] = listOf(MatchInterest("a", "pa"), MatchInterest("d", "pd"))
        store.load(MatchBox.INCOMING)
        store.accept("a")
        assertEquals(listOf("d"), store.items(MatchBox.INCOMING)!!.map { it.uid })
        assertEquals(listOf("a"), store.items(MatchBox.MATCHES)!!.map { it.uid })
    }

    @Test
    fun declineWithdrawUnmatchRemoveAndRollBackOnFailure() = runTest {
        client.lists[MatchBox.INCOMING] = listOf(MatchInterest("a"))
        client.lists[MatchBox.OUTGOING] = listOf(MatchInterest("b"))
        client.lists[MatchBox.MATCHES] = listOf(MatchInterest("c", status = "matched"))
        MatchBox.entries.forEach { store.load(it) }

        client.fail = IOException("offline")
        runCatching { store.unmatch("c") }
        assertEquals(InterestState.MATCHED, store.state("c")) // 戻る

        client.fail = null
        store.decline("a")
        store.withdraw("b")
        store.unmatch("c")
        MatchBox.entries.forEach { assertEquals(emptyList<MatchInterest>(), store.items(it)) }
        assertTrue(client.calls.containsAll(listOf("decline a", "decline b", "unmatch c")))
    }

    @Test
    fun failedFirstLoadMarksTheBoxButKeepsAnEarlierList() = runTest {
        client.fail = IOException("offline")
        runCatching { store.load(MatchBox.MATCHES) }
        assertTrue(MatchBox.MATCHES in store.snapshot.value.failed)

        client.fail = null
        client.lists[MatchBox.MATCHES] = listOf(MatchInterest("c"))
        store.load(MatchBox.MATCHES)
        assertFalse(MatchBox.MATCHES in store.snapshot.value.failed)

        client.fail = IOException("offline")
        runCatching { store.load(MatchBox.MATCHES) }
        assertFalse(MatchBox.MATCHES in store.snapshot.value.failed) // 前の一覧があれば出し続ける
        assertEquals(listOf("c"), store.items(MatchBox.MATCHES)!!.map { it.uid })
    }

    @Test
    fun forgetRemovesABlockedPersonEverywhere() = runTest {
        client.lists[MatchBox.INCOMING] = listOf(MatchInterest("x"))
        client.lists[MatchBox.MATCHES] = listOf(MatchInterest("x", status = "matched"))
        MatchBox.entries.forEach { store.load(it) }
        store.forget("x")
        assertEquals(InterestState.NONE, store.state("x"))
    }

    @Test
    fun ageGateIsRememberedAndCanBeReset() {
        val kv = InMemoryKeyValueStore()
        val gate = MatchingAgeGate(kv)
        assertFalse(gate.isConfirmed)
        gate.confirm()
        assertTrue(MatchingAgeGate(kv).isConfirmed) // 端末に覚える(鍵は iOS と同じ)
        assertEquals(true, kv.getBoolean("matching.ageConfirmed.v1"))
        gate.reset()
        assertFalse(gate.isConfirmed)
    }
}
