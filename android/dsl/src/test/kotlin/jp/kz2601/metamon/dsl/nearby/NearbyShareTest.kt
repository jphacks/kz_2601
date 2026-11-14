package jp.kz2601.metamon.dsl.nearby

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.nearby.NearbyShare.Decision
import jp.kz2601.metamon.dsl.nearby.NearbyShare.FieldSignature
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.Fixtures
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.dsl.records.entityModels
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Test

/** `nearbyShare`(#36)の送受信データ・取り込み判定。iOS の NearbyShareTests と同じ場合分け。 */
class NearbyShareTest {
    private fun load(name: String): AppDSL = AppDSL.decode(Fixtures.examplesDir.resolve("$name.json").readText())

    private fun attendance(): Pair<AppDSL, Entity> {
        val dsl = load("attendance_nearby")
        return dsl to dsl.entityModels().first { it.id == "attendance" }
    }

    private fun rec(vararg p: Pair<String, Any?>): Map<String, JSONValue> = p.associate { it.first to JSONValue.of(it.second) }

    @Test fun exampleUsesNearbyShareOnFormAndList() {
        val dsl = load("attendance_nearby")
        val shares = dsl.screens.flatMap { s -> s.components.filter { it.type == ComponentType.NEARBY_SHARE }.map { s to it } }
        assertEquals(2, shares.size)
        assertTrue("フォーム画面の nearbyShare", shares.any { it.first.entity == it.second.entity })
        assertTrue("一覧画面の nearbyShare", shares.any { it.first.entity == null && it.second.entity == "members" })
        assertTrue(shares.all { it.second.label?.isNotEmpty() == true })
    }

    @Test fun payloadRoundTripDropsUnknownAndNull() {
        val (_, entity) = attendance()
        val payload = NearbyShare.makePayload(
            "花子", "app1", "出席カード", entity,
            listOf(rec("name" to "花子", "status" to "出席", "secret" to "x", "date" to null), rec("secret" to "only unknown")),
        )
        assertEquals(1, payload.header.count)
        assertEquals(listOf(rec("name" to "花子", "status" to "出席")), payload.records)
        assertEquals(setOf("name", "status", "date"), payload.header.fields.map { it.id }.toSet())

        assertEquals(payload, NearbyShare.decodePayload(NearbyShare.encode(payload)))
    }

    @Test fun decodeIgnoresUnknownTopLevelKeys() {
        val json = """
            {"header":{"v":1,"sender":"太郎","appId":"a","appName":"x","entityId":"attendance","entityName":"出席",
              "fields":[{"id":"name","type":"string"}],"count":1,"future":true},
             "records":[{"name":"太郎"}],"extra":[1,2]}
        """.trimIndent()
        val p = NearbyShare.decodePayload(json.toByteArray()) ?: return fail("読めるはず")
        assertEquals("太郎", p.header.sender)
        assertEquals(JSONValue.Str("太郎"), p.records.first()["name"])
    }

    @Test fun decodeRejectsMissingOrWrongTypedKeys() {
        val bad = listOf(
            """{"header":{"v":1,"sender":"a","appId":"a","appName":"x","entityId":"e","entityName":"e","fields":[]},"records":[]}""",
            """{"header":{"v":"1","sender":"a","appId":"a","appName":"x","entityId":"e","entityName":"e","fields":[],"count":0},"records":[]}""",
            """{"header":{"v":1,"sender":"a","appId":"a","appName":"x","entityId":"e","entityName":"e","fields":[],"count":0},"records":[1]}""",
            """{"header":{"v":1,"sender":"a","appId":"a","appName":"x","entityId":"e","entityName":"e","fields":[],"count":0.5},"records":[]}""",
            "not json",
        )
        for (json in bad) assertNull(json, NearbyShare.decodePayload(json.toByteArray()))
    }

    @Test fun encodeMatchesIosSortedKeysAndEscaping() {
        val (_, entity) = attendance()
        val p = NearbyShare.makePayload("a/b", "app1", "出席", entity, listOf(rec("name" to "x\"y", "status" to "出席")))
        val json = String(NearbyShare.encode(p))
        assertEquals(
            """{"header":{"appId":"app1","appName":"出席","count":1,"entityId":"attendance","entityName":"出席",""" +
                """"fields":[{"id":"name","type":"string"},{"id":"status","type":"enum"},{"id":"date","type":"datetime"}],""" +
                """"sender":"a\/b","v":1},"records":[{"name":"x\"y","status":"出席"}]}""",
            json,
        )
        assertEquals("""{"added":2,"ok":true}""", String(NearbyShare.encode(NearbyShare.Ack(true, 2))))
        assertEquals(NearbyShare.Ack(false, 0, "x"), NearbyShare.decodeAck(NearbyShare.encode(NearbyShare.Ack(false, 0, "x"))))
    }

    @Test fun decideSameAppAcceptsEvenIfFieldsDiffer() {
        val (dsl, entity) = attendance()
        val h = NearbyShare.Header.of("s", "app1", "x", entity, 1).copy(fields = listOf(FieldSignature("name", "string")))
        assertEquals(Decision.Accept("attendance", sameApp = true), NearbyShare.decide(h, dsl, "app1"))
    }

    @Test fun decideOtherAppNeedsSameFields() {
        val (dsl, entity) = attendance()
        val h = NearbyShare.Header.of("s", "other", "x", entity, 1)
        assertEquals(Decision.Accept("attendance", sameApp = false), NearbyShare.decide(h, dsl, "app1"))

        val changed = h.copy(fields = h.fields + FieldSignature("room", "string"))
        val r = NearbyShare.decide(changed, dsl, "app1") as? Decision.Reject ?: return fail("reject 期待")
        assertTrue(r.reason, r.reason.contains("項目が違う"))

        val typeChanged = h.copy(fields = h.fields.map { if (it.id == "date") FieldSignature("date", "string") else it })
        assertNotEquals(Decision.Accept("attendance", sameApp = false), NearbyShare.decide(typeChanged, dsl, "app1"))
    }

    @Test fun decideMissingEntityRejects() {
        val (_, entity) = attendance()
        val kakeibo = load("kakeibo")
        val h = NearbyShare.Header.of("s", "app1", "x", entity, 3)
        val r = NearbyShare.decide(h, kakeibo, "app1") as? Decision.Reject ?: return fail("reject 期待")
        assertTrue(r.reason, r.reason.contains("「出席」がない"))
    }

    @Test fun decideRejectsNewerVersion() {
        val (dsl, entity) = attendance()
        val h = NearbyShare.Header.of("s", "app1", "x", entity, 1).copy(v = NearbyShare.VERSION + 1)
        assertTrue(NearbyShare.decide(h, dsl, "app1") is Decision.Reject)
    }

    @Test fun sanitizeDropsUnknownWrongTypeAndCrossAppRef() {
        val warikan = load("warikan").entityModels()
        val entity = warikan.first { e -> e.storedFields.any { it.type == jp.kz2601.metamon.dsl.records.FieldType.REF } }
        val ref = entity.storedFields.first { it.type == jp.kz2601.metamon.dsl.records.FieldType.REF }
        val num = entity.storedFields.firstOrNull { it.type.isNumeric }
        val row = buildMap<String, JSONValue> {
            put(ref.id, JSONValue.Str("r1"))
            put("unknown", JSONValue.Str("x"))
            num?.let { put(it.id, JSONValue.Str("not a number")) }
        }
        assertEquals("ref は別アプリだと捨てる・型違いも捨てる", emptyList<Map<String, JSONValue>>(), NearbyShare.sanitize(listOf(row), entity, sameApp = false))
        assertEquals(listOf(mapOf(ref.id to JSONValue.Str("r1"))), NearbyShare.sanitize(listOf(row), entity, sameApp = true))
    }

    @Test fun recordsToSendNewestFirstAndCapped() {
        val records = (0 until 60).map { i ->
            Record(id = "$i", data = mapOf("n" to JSONValue.number(i)), createdAt = "2026-10-24T10:%02d:00Z".format(i))
        }
        val out = NearbyShare.recordsToSend(records)
        assertEquals(NearbyShare.MAX_SEND_RECORDS, out.size)
        assertEquals(JSONValue.number(59), out.first()["n"])
    }

    @Test fun receiveCap() {
        val (_, entity) = attendance()
        val many = List(NearbyShare.MAX_RECEIVE_RECORDS + 10) { rec("name" to "a") }
        assertEquals(NearbyShare.MAX_RECEIVE_RECORDS, NearbyShare.sanitize(many, entity, sameApp = true).size)
    }

    @Test fun songRequestNeedsTitleAndAllowRequests() {
        val song = MusicSong(id = " 123.a ", title = "  曲  ", artist = "人", artworkURL = "http://x/y.png")
        val p = NearbyShare.makeSongRequest(song, "s", "app1", "x") ?: return fail("作れるはず")
        assertEquals(MusicSong(id = "123.a", title = "曲", artist = "人", artworkURL = null), p.header.song)
        assertEquals(NearbyShare.SONG_ENTITY_ID, p.header.entityId)
        assertTrue(p.records.isEmpty())
        assertNull(NearbyShare.makeSongRequest(song.copy(title = "   "), "s", "app1", "x"))
        assertEquals(p, NearbyShare.decodePayload(NearbyShare.encode(p)))

        assertEquals(Decision.Accept(NearbyShare.SONG_ENTITY_ID, sameApp = true), NearbyShare.decide(p.header, emptyList(), true, "app1"))
        assertTrue(NearbyShare.decide(p.header, emptyList(), false, "app1") is Decision.Reject)
        val noTitle = p.header.copy(song = song.copy(title = ""))
        assertTrue(NearbyShare.decide(noTitle, emptyList(), true, "app1") is Decision.Reject)
    }

    @Test fun songSanitizeLimits() {
        val s = MusicRequestRules.sanitize(
            MusicSong(id = "a-b", title = "あ".repeat(150), artist = "x", artworkURL = "https://" + "a".repeat(600)),
        ) ?: return fail("曲名があるので残る")
        assertNull("英数字とドット以外の id は捨てる", s.id)
        assertEquals(MusicRequestRules.MAX_TITLE, s.title.length)
        assertNull("長すぎるジャケット URL は捨てる", s.artworkURL)
        assertEquals("https://e.com/a.png", MusicRequestRules.sanitize(MusicSong(title = "t", artist = "", artworkURL = "https://e.com/a.png"))?.artworkURL)
        assertNull(MusicRequestRules.sanitize(MusicSong(id = "a".repeat(41), title = "t", artist = ""))?.id)
    }

    @Test fun usesMusicRequestsLooksAtFirstPlayer() {
        val on = AppDSL.decode(
            """{"schemaVersion":1,"app":{"name":"x","voiceEntry":"none"},"initialScreen":"h",
               "screens":[{"id":"h","title":"h","components":[{"type":"musicPlayer","allowRequests":true}]}]}""",
        )
        assertTrue(on.usesMusicRequests)
        assertTrue(!load("attendance_nearby").usesMusicRequests)
    }

    @Test fun receiveByteAndLengthLimits() {
        val (_, entity) = attendance()
        val payload = NearbyShare.makePayload("花子", "app1", "出席カード", entity, listOf(rec("name" to "花子")))
        assertEquals(payload, NearbyShare.decodePayload(NearbyShare.encode(payload)))
        assertNull("大きすぎるデータは読まない", NearbyShare.decodePayload(ByteArray(NearbyShare.MAX_RECEIVE_BYTES + 1) { 0x20 }))
        assertNull(NearbyShare.decodeHeader(ByteArray(NearbyShare.MAX_HEADER_BYTES + 1) { 0x20 }))

        val tooMany = payload.copy(records = List(NearbyShare.MAX_RECEIVE_RECORDS + 1) { rec("name" to "a") })
        assertNull("受け取る件数の上限を超えたら捨てる", NearbyShare.decodePayload(NearbyShare.encode(tooMany)))

        val h = payload.header.copy(sender = "名".repeat(200), appName = "a".repeat(300), count = 10_000)
        val v = NearbyShare.decodeHeader(NearbyShare.encode(h)) ?: return fail("読めるはず")
        assertEquals("名前は切り詰める", NearbyLimits.MAX_NAME_LENGTH, v.sender.length)
        assertEquals(NearbyLimits.MAX_LABEL_LENGTH, v.appName.length)
        assertEquals(NearbyShare.MAX_RECEIVE_RECORDS, v.count)

        val longEntity = payload.header.copy(entityId = "e".repeat(NearbyLimits.MAX_ID_LENGTH + 1))
        assertNull("id が長すぎる申し出は断る", NearbyShare.decodeHeader(NearbyShare.encode(longEntity)))
        val manyFields = payload.header.copy(fields = (0..NearbyLimits.MAX_FIELDS).map { FieldSignature("f$it", "string") })
        assertNull("項目一覧が長すぎる申し出は断る", NearbyShare.decodeHeader(NearbyShare.encode(manyFields)))
        val anonymous = payload.header.copy(sender = "")
        assertEquals(NearbyLimits.ANONYMOUS_NAME, NearbyShare.decodeHeader(NearbyShare.encode(anonymous))?.sender)

        val ack = NearbyShare.Ack(ok = true, added = 9_999, message = "m".repeat(1000))
        val a = NearbyShare.decodeAck(NearbyShare.encode(ack)) ?: return fail("読めるはず")
        assertEquals(NearbyShare.MAX_RECEIVE_RECORDS, a.added)
        assertEquals(NearbyLimits.MAX_MESSAGE_LENGTH, a.message?.length)
        assertNull(NearbyShare.decodeAck(ByteArray(NearbyShare.MAX_ACK_BYTES + 1) { 0x20 }))
    }

    @Test fun sendStaysUnderByteLimit() {
        val (_, entity) = attendance()
        val big = "あ".repeat(4000) // 約 12 KB / 件
        val rows = List(NearbyShare.MAX_SEND_RECORDS) { rec("name" to big) }
        val p = NearbyShare.makePayload("s", "a", "x", entity, rows)
        assertTrue(NearbyShare.encode(p).size <= NearbyShare.MAX_SEND_BYTES)
        assertTrue(p.records.size < NearbyShare.MAX_SEND_RECORDS)
        assertEquals(p.records.size, p.header.count)
        assertTrue("受け取る上限は送る上限より大きい", NearbyShare.MAX_SEND_BYTES < NearbyShare.MAX_RECEIVE_BYTES)
    }

    /** 「名前を出さない」(すれ違い通信と同じ設定): 端末の名前と Header の送り主が「近くの人」になり、オフなら本名。 */
    @Test fun anonymousSenderAndPeerName() {
        val (_, entity) = attendance()
        val rows = listOf(rec("name" to "花子"))

        val anonPeer = NearbyLimits.peerName("花子のPixel", anonymous = true)
        assertEquals(NearbyLimits.ANONYMOUS_NAME, anonPeer)
        val anon = NearbyShare.makePayload(anonPeer, "a", "x", entity, rows)
        assertEquals(NearbyLimits.ANONYMOUS_NAME, anon.header.sender)
        val anonHeader = NearbyShare.decodeHeader(NearbyShare.encode(anon.header))
        assertEquals("受け取った側でも決まった文言のまま", NearbyLimits.ANONYMOUS_NAME, anonHeader?.sender)
        assertEquals("近くの人", NearbyLimits.displayName(anonHeader?.sender ?: ""))

        val realPeer = NearbyLimits.peerName(" 花子のPixel ", anonymous = false)
        assertEquals("花子のPixel", realPeer)
        val real = NearbyShare.makePayload(realPeer, "a", "x", entity, rows)
        assertEquals("花子のPixel", real.header.sender)

        assertEquals("送る直前に今の名前へ差し替える", NearbyLimits.ANONYMOUS_NAME, real.sentAs(anonPeer).header.sender)
        assertEquals(real.records, real.sentAs(anonPeer).records)
        assertEquals("空なら「近くの人」", NearbyLimits.ANONYMOUS_NAME, NearbyLimits.peerName("", anonymous = false))
        val long = NearbyLimits.peerName("あ".repeat(60), anonymous = false)
        assertTrue("端末の名前の上限(UTF-8 63 バイト)", long.toByteArray(Charsets.UTF_8).size <= 63)
        assertTrue(long.startsWith("あ"))
        assertEquals("書記素の途中で切らない", 21, long.length)
    }
}
