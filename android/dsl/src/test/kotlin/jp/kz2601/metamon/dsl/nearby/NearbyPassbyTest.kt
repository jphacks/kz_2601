package jp.kz2601.metamon.dsl.nearby

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.nearby.NearbyPassby.Config
import jp.kz2601.metamon.dsl.nearby.NearbyPassby.Item
import jp.kz2601.metamon.dsl.nearby.NearbyPassby.Ledger
import jp.kz2601.metamon.dsl.nearby.NearbyPassby.Plan
import jp.kz2601.metamon.dsl.nearby.NearbyPassby.ToggleOutcome
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.dsl.records.entityModels
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.Instant

/** すれ違い通信(nearbyShare mode:passby・#425)。iOS の NearbyPassbyTests と同じ場合分け。 */
class NearbyPassbyTest {
    private val json = """
        {"schemaVersion":1,"app":{"name":"すれ違い","voiceEntry":"none"},"initialScreen":"home",
         "entities":[{"id":"profile","name":"プロフィール","fields":[
           {"id":"name","name":"名前","type":"string","required":true},
           {"id":"hobby","name":"趣味","type":"string"},
           {"id":"age","name":"年齢","type":"integer"},
           {"id":"secret","name":"メモ","type":"string","hidden":true},
           {"id":"photo","name":"写真","type":"photo"},
           {"id":"len","name":"文字数","type":"integer","computed":{"op":"add","args":[{"ref":"age"},1]}}]}],
         "screens":[{"id":"home","title":"すれ違い","components":[
           {"type":"nearbyShare","id":"pb","label":"すれ違い通信","entity":"profile","mode":"passby",
            "fields":["name","hobby","age","secret","photo","len"],"limit":2,"distance":true},
           {"type":"nearbyShare","label":"送る","entity":"profile"}]}]}
    """.trimIndent()

    private fun fixture(): Triple<AppDSL, Entity, Config> {
        val dsl = AppDSL.decode(json)
        val entity = dsl.entityModels().first { it.id == "profile" }
        return Triple(dsl, entity, Config.of(dsl.screens[0].components[0])!!)
    }

    private fun d(vararg p: Pair<String, Any?>): Map<String, JSONValue> = p.associate { it.first to JSONValue.of(it.second) }

    @Test fun configOnlyForPassby() {
        val (dsl, _, config) = fixture()
        assertEquals("pb", config.componentId)
        assertEquals(2, config.limit)
        assertTrue(config.distance)
        assertNull("send の nearbyShare は対象外", Config.of(dsl.screens[0].components[1]))
        assertEquals(20, Config("x", "e", "", emptyList(), limit = 99).limit)
        assertEquals(1, Config("x", "e", "", emptyList(), limit = 0).limit)
    }

    @Test fun allowedFieldsDropHiddenMediaAndComputed() {
        val (_, entity, config) = fixture()
        assertEquals(listOf("name", "hobby", "age"), NearbyPassby.allowedFields(config, entity).map { it.id })
    }

    @Test fun payloadSendsOnlyMyNewestRecordsWithListedFields() {
        val (_, entity, config) = fixture()
        val ledger = Ledger()
        ledger.commit(Ledger.key("s2", "r9"), d("name" to "花子"))
        val records = listOf(
            Record("old", d("name" to "古い"), createdAt = "2026-10-01T00:00:00Z"),
            Record("new", d("name" to "太郎", "hobby" to "釣り", "secret" to "x", "photo" to "data:..", "len" to 2, "age" to null), createdAt = "2026-10-28T00:00:00Z"),
            Record("got", d("name" to "花子"), createdAt = "2026-10-29T00:00:00Z"), // 受け取った記録
            Record("theirs", d("name" to "他人"), createdAt = "2026-10-30T00:00:00Z", author = "u2"), // 他人の共有レコード
            Record("mine", d("name" to "自分"), createdAt = "2026-10-27T00:00:00Z", author = "me"),
        )
        val p = NearbyPassby.makePayload("s1", "太郎", entity, config, records, ledger, me = "me")
        assertEquals("新しい順に limit 件・受け取った/他人の記録は除く", listOf("new", "mine"), p.records.map { it.id })
        assertEquals("hidden・写真・computed・null は送らない", d("name" to "太郎", "hobby" to "釣り"), p.records[0].data)
        assertEquals("profile", p.entityId)
    }

    @Test fun payloadStaysUnder16KB() {
        val (_, entity, _) = fixture()
        val config = Config("pb", "profile", "", listOf("name", "hobby"), limit = 20)
        val big = "あ".repeat(1500) // 約 4.5 KB / 件
        val records = (0 until 30).map { Record("r$it", d("name" to "n$it", "hobby" to big)) }
        val p = NearbyPassby.makePayload("s", "n", entity, config, records, Ledger(), me = null)
        assertTrue(NearbyPassby.encode(p).size <= NearbyPassby.MAX_BYTES)
        assertEquals(3, p.records.size)

        val small = (0 until 30).map { Record("r$it", d("name" to "n$it")) }
        val q = NearbyPassby.makePayload("s", "n", entity, config, small, Ledger(), me = null)
        assertEquals(NearbyPassby.MAX_RECORDS, q.records.size)
    }

    @Test fun sizeCountsSlashEscapeLikeIos() {
        val (_, entity, _) = fixture()
        val config = Config("pb", "profile", "", listOf("name"), limit = 1)
        // "/" は iOS の JSONEncoder で "\/"(2 バイト)。16 KB の判定もそれで数える
        val p = NearbyPassby.makePayload("s", "n", entity, config, listOf(Record("r", d("name" to "/".repeat(9000)))), Ledger(), me = null)
        assertTrue("2 バイトで数えると 16 KB を超えるので送らない", p.records.isEmpty())
    }

    @Test fun sanitizeKeepsOnlyMyAllowedFields() {
        val (_, entity, config) = fixture()
        val rows = (0 until 25).map { Item("r$it", d("name" to "花子")) }
        val payload = NearbyPassby.Payload(
            1, "s2", "花子", "profile",
            listOf(
                Item("a", d("name" to "花子", "secret" to "答え", "photo" to "data:x", "age" to "二十", "unknown" to 1)),
                Item("b", d("secret" to "only")),
                Item("", d("name" to "id 無し")),
            ) + rows,
        )
        val items = NearbyPassby.sanitize(payload, entity, config)
        assertEquals("未許可・型違いは捨てる", Item("a", d("name" to "花子")), items.first())
        assertEquals("空の行・id 無しを除き、先頭 20 件まで", 18, items.size)
        assertTrue(NearbyPassby.sanitize(payload.copy(entityId = "memo"), entity, config).isEmpty())
    }

    @Test fun ledgerDedupesBySenderAndRecordId() {
        val ledger = Ledger()
        assertEquals(listOf(Plan.Create("s2|r1", d("name" to "花子"))), ledger.plan(listOf(Item("r1", d("name" to "花子"))), "s2"))
        ledger.commit("s2|r1", d("name" to "花子"))

        assertEquals("同じ相手・同じ記録は 2 度追加しない", listOf(Plan.Skip("s2|r1")), ledger.plan(listOf(Item("r1", d("name" to "花子"))), "s2"))
        assertEquals(
            "変わったら更新",
            listOf(Plan.Update("s2|r1", d("name" to "花子"), d("name" to "はなこ"))),
            ledger.plan(listOf(Item("r1", d("name" to "はなこ"))), "s2"),
        )
        assertEquals("別の相手の同じ id は別物", listOf(Plan.Create("s3|r1", d("name" to "花子"))), ledger.plan(listOf(Item("r1", d("name" to "花子"))), "s3"))

        val local = listOf(Record("tmp", d("name" to "花子", "note" to "自分で足したメモ")))
        assertEquals("仮 id が変わっても内容で探す", "tmp", Ledger.match(d("name" to "花子"), local)?.id)
        assertTrue(ledger.isReceived(d("name" to "花子", "hobby" to "x")))
        assertFalse(ledger.isReceived(d("name" to "太郎")))
    }

    @Test fun ledgerReceivedCapDropsSmallestKey() {
        val ledger = Ledger()
        for (i in 0..Ledger.MAX_RECEIVED) ledger.commit("s|%04d".format(i), d("n" to i))
        assertEquals(Ledger.MAX_RECEIVED, ledger.received.size)
        assertFalse(ledger.received.containsKey("s|0000"))
    }

    @Test fun encountersAndCooldown() {
        val ledger = Ledger()
        val t0 = Instant.ofEpochSecond(1_000_000)
        assertTrue(ledger.shouldExchange("s2", t0))
        ledger.met("s2", "花子", t0, 1)
        assertFalse("続けて交換しない", ledger.shouldExchange("s2", t0.plusSeconds(60)))
        ledger.met("s3", "次郎", t0.plusSeconds(30), 0)
        ledger.met("s2", "花子さん", t0.plus(NearbyPassby.COOLDOWN).plusSeconds(1), 1)
        assertEquals("最後に会った順", listOf("s2", "s3"), ledger.encounters.map { it.id })
        val e = ledger.encounters[0]
        assertEquals(2, e.times)
        assertEquals(2, e.records)
        assertEquals("花子さん", e.name)
        assertEquals(t0, e.firstMet)

        val data = NearbyPassby.encode(ledger)
        assertTrue("日時は iOS の .iso8601 と同じ形", String(data).contains("\"firstMet\":\"1970-01-12T13:46:40Z\""))
        assertEquals(ledger, NearbyPassby.decodeLedger(data))

        for (i in 0 until Ledger.MAX_ENCOUNTERS + 5) ledger.met("x$i", "x", t0, 0)
        assertEquals(Ledger.MAX_ENCOUNTERS, ledger.encounters.size)
    }

    @Test fun consentBeforeFirstAdvertising() {
        assertEquals(ToggleOutcome.ASK_CONSENT, NearbyPassby.toggle(true, null))
        assertEquals(ToggleOutcome.ASK_CONSENT, NearbyPassby.toggle(true, 0))
        assertEquals(ToggleOutcome.START, NearbyPassby.toggle(true, NearbyPassby.CONSENT_VERSION))
        assertEquals(ToggleOutcome.STOP, NearbyPassby.toggle(false, null))
        val text = NearbyPassby.consentText(listOf("名前", "趣味"))
        assertTrue(text.contains("「名前」「趣味」"))
        assertTrue(text.contains("裏に回すと止まります"))
    }

    @Test fun appTagAndFormatting() {
        assertEquals(NearbyPassby.appTag("app1"), NearbyPassby.appTag("app1"))
        assertNotEquals(NearbyPassby.appTag("app1"), NearbyPassby.appTag("app2"))
        assertFalse("appId 自体は流さない", NearbyPassby.appTag("app1").contains("app1"))
        // FNV-1a 64bit の既知の値(iOS の appTag と同じ)
        assertEquals("cbf29ce484222325", NearbyPassby.appTag(""))
        assertEquals("af63dc4c8601ec8c", NearbyPassby.appTag("a"))
        assertEquals("85944171f73967e8", NearbyPassby.appTag("foobar"))
        assertEquals("0.8 m", NearbyPassby.distanceText(0.84f))
        assertEquals("12 m", NearbyPassby.distanceText(12.4f))
        assertEquals("13 m", NearbyPassby.distanceText(12.5f))
        assertEquals("正面", NearbyPassby.directionText(0f))
        assertEquals("右前", NearbyPassby.directionText((Math.PI / 4).toFloat()))
        assertEquals("左", NearbyPassby.directionText((-Math.PI / 2).toFloat()))
    }

    @Test fun helloRoundTrip() {
        val hello = NearbyPassby.Hello(app = NearbyPassby.appTag("a"), senderId = "s1", name = "太郎")
        assertEquals(hello, NearbyPassby.decodeHello(NearbyPassby.encode(hello)))
        assertEquals("""{"app":"af63dc4c8601ec8c","name":"太郎","senderId":"s1","v":1}""", String(NearbyPassby.encode(hello)))
    }

    @Test fun payloadDecodesIosJsonAndRejectsMissingData() {
        val p = NearbyPassby.decodePayload(
            """{"entityId":"profile","name":"花子","records":[{"data":{"name":"花子"},"id":"a","x":1}],"senderId":"s2","v":1,"y":2}""".toByteArray(),
        )
        assertEquals(listOf(Item("a", d("name" to "花子"))), p.records)
        assertTrue(runCatching { NearbyPassby.decodePayload("""{"entityId":"p","name":"n","records":[{"id":"a"}],"senderId":"s","v":1}""".toByteArray()) }.isFailure)
    }
}
