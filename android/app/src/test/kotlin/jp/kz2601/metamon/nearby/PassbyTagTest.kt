package jp.kz2601.metamon.nearby

import jp.kz2601.metamon.dsl.nearby.NearbyLimits
import jp.kz2601.metamon.dsl.nearby.NearbyPassby
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** すれ違い通信の見つけ合う印(iOS の discoveryInfo `v`・`a`・`s`)と、申し出る側の決め方。 */
class PassbyTagTest {
    private val app = NearbyPassby.appTag("app1")
    private val me = NearbyPassby.Hello(app = app, senderId = "bbb", name = "太郎")

    @Test fun roundTripWithoutName() {
        val tag = PassbyTag.encode(me)
        assertFalse("名前は載せない", "太郎" in tag)
        val back = PassbyTag.decode(tag)!!
        assertEquals(app, back.app)
        assertEquals("bbb", back.senderId)
        assertEquals(NearbyLimits.ANONYMOUS_NAME, back.name)
    }

    @Test fun rejectsBrokenOrTooLong() {
        assertNull(PassbyTag.decode(null))
        assertNull(PassbyTag.decode("1|$app"))
        assertNull(PassbyTag.decode("x|$app|bbb"))
        assertNull(PassbyTag.decode("1||bbb"))
        assertNull("id が長すぎる", PassbyTag.decode("1|$app|${"a".repeat(NearbyLimits.MAX_ID_LENGTH + 1)}"))
        assertNull("1 KB を超える", PassbyTag.decode("1|$app|" + "a".repeat(NearbyPassby.MAX_HELLO_BYTES)))
    }

    @Test fun onlyTheSmallerSenderInvites() {
        val bigger = NearbyPassby.Hello(app = app, senderId = "ccc", name = "")
        val smaller = NearbyPassby.Hello(app = app, senderId = "aaa", name = "")
        assertTrue(PassbyTag.shouldInvite(me, bigger) { true })
        assertFalse("相手の方が小さい = 相手が申し出る", PassbyTag.shouldInvite(me, smaller) { true })
        assertFalse("cooldown の中", PassbyTag.shouldInvite(me, bigger) { false })
        assertFalse("別のアプリ", PassbyTag.shouldInvite(me, bigger.copy(app = NearbyPassby.appTag("app2"))) { true })
        assertFalse("自分", PassbyTag.shouldInvite(me, me) { true })
    }

    @Test fun acceptsSameAppOutsideCooldown() {
        val other = NearbyPassby.Hello(app = app, senderId = "aaa", name = "")
        assertTrue(PassbyTag.shouldAccept(me, other) { true })
        assertFalse(PassbyTag.shouldAccept(me, other) { false })
        assertFalse(PassbyTag.shouldAccept(me, other.copy(app = "zzz")) { true })
        assertFalse(PassbyTag.shouldAccept(me, me) { true })
    }
}
