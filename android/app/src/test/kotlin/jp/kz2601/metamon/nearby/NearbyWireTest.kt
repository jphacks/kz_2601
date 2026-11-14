package jp.kz2601.metamon.nearby

import jp.kz2601.metamon.dsl.nearby.NearbyShare
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** 1 通を 32 KB 以内の切れに分けて、受ける側で戻す(#1382)。 */
class NearbyWireTest {
    @Test fun splitsIntoChunksUnderTheLimitAndJoins() {
        val body = ByteArray(100_000) { (it % 251).toByte() }
        val frames = NearbyWire.frames(NearbyWire.Kind.PAYLOAD, body)
        assertEquals(4, frames.size)
        assertTrue(frames.all { it.size <= NearbyWire.MAX_CHUNK_BYTES })
        val reader = NearbyWire.Reader()
        val out = frames.mapNotNull { reader.accept(it) }
        assertEquals(1, out.size)
        assertEquals(NearbyWire.Kind.PAYLOAD, out.single().kind)
        assertArrayEquals(body, out.single().body)
    }

    @Test fun emptyBodyIsOneFrame() {
        val frames = NearbyWire.frames(NearbyWire.Kind.READY, ByteArray(0))
        assertEquals(1, frames.size)
        assertEquals(NearbyWire.Message(NearbyWire.Kind.READY, ByteArray(0)), NearbyWire.Reader().accept(frames.single()))
    }

    @Test fun readsSeveralMessagesInOrder() {
        val reader = NearbyWire.Reader()
        val a = NearbyWire.frames(NearbyWire.Kind.HEADER, "h".toByteArray()).mapNotNull { reader.accept(it) }
        val b = NearbyWire.frames(NearbyWire.Kind.ACK, "a".toByteArray()).mapNotNull { reader.accept(it) }
        assertEquals(listOf(NearbyWire.Kind.HEADER, NearbyWire.Kind.ACK), (a + b).map { it.kind })
    }

    @Test fun overTheKindLimitBreaksTheReader() {
        val reader = NearbyWire.Reader()
        val frames = NearbyWire.frames(NearbyWire.Kind.HEADER, ByteArray(NearbyShare.MAX_HEADER_BYTES + 1))
        assertTrue(frames.all { reader.accept(it) == null })
        assertTrue(reader.broken)
        assertNull("壊れた後は読まない", reader.accept(NearbyWire.frames(NearbyWire.Kind.READY, ByteArray(0)).single()))
    }

    @Test fun unknownKindOrMixedKindsBreakTheReader() {
        val unknown = NearbyWire.Reader()
        assertNull(unknown.accept(byteArrayOf('Z'.code.toByte(), 1)))
        assertTrue(unknown.broken)

        val mixed = NearbyWire.Reader()
        val first = NearbyWire.frames(NearbyWire.Kind.PAYLOAD, ByteArray(40_000)).first()
        assertNull(mixed.accept(first))
        assertFalse(mixed.broken)
        assertNull(mixed.accept(NearbyWire.frames(NearbyWire.Kind.ACK, ByteArray(1)).single()))
        assertTrue("途中で別の種類が来たら読まない", mixed.broken)
        assertNull(NearbyWire.Reader().accept(byteArrayOf(1)))
    }
}
