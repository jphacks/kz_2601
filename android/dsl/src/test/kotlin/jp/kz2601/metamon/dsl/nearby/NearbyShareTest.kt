package jp.kz2601.metamon.dsl.nearby

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.nearby.NearbyShare.Decision
import jp.kz2601.metamon.dsl.nearby.NearbyShare.FieldSignature
import jp.kz2601.metamon.dsl.records.DslDecodeException
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
        val p = NearbyShare.decodePayload(json.toByteArray())
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
        for (json in bad) {
            try {
                NearbyShare.decodePayload(json.toByteArray())
                fail("例外のはず: $json")
            } catch (_: DslDecodeException) {
            }
        }
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
}
