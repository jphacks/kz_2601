package jp.kz2601.metamon.dsl.nearby

import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentMode
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.Field
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import java.time.Duration
import java.time.Instant
import java.util.Locale
import java.util.UUID
import kotlin.math.abs
import kotlin.math.floor
import kotlin.math.sign

/**
 * `nearbyShare` mode:passby(すれ違い通信・#425)の純粋ロジック。通信はアプリ本体。iOS の `NearbyPassby`。
 *
 * - 送る: `fields` に明示した保存 field だけ。hidden(#262)・photo・video・ref・refs・computed は送らない
 * - 量: 1 回 `limit` 件(既定 1・最大 20 件)かつ JSON で 16 KB 以内
 * - 受け取り: 同じ相手(senderId)・同じ記録 id は 2 度追加しない。内容が変わっていれば更新する
 */
object NearbyPassby {
    const val VERSION = 1
    const val MAX_RECORDS = 20
    const val MAX_BYTES = 16 * 1024
    const val DEFAULT_LIMIT = 1

    /** 同じ相手と続けて交換しない間隔(離れて戻ってきたら「また会った」)。 */
    val COOLDOWN: Duration = Duration.ofMinutes(10)

    /** 同意文を変えたら上げる(もう一度同意を取る)。 */
    const val CONSENT_VERSION = 1

    val BLOCKED_TYPES: Set<FieldType> = setOf(FieldType.PHOTO, FieldType.VIDEO, FieldType.REF, FieldType.REFS)

    // MARK: - 設定

    /** `limit` は 1〜[MAX_RECORDS] に丸める。 */
    class Config(
        val componentId: String,
        val entityId: String,
        val label: String,
        val fields: List<String>,
        limit: Int = DEFAULT_LIMIT,
        val distance: Boolean = false,
    ) {
        val limit: Int = limit.coerceIn(1, MAX_RECORDS)

        override fun equals(other: Any?): Boolean = other is Config && componentId == other.componentId &&
            entityId == other.entityId && label == other.label && fields == other.fields && limit == other.limit && distance == other.distance

        override fun hashCode(): Int = listOf(componentId, entityId, label, fields, limit, distance).hashCode()

        companion object {
            /** passby の nearbyShare なら設定を返す。id の無い部品は iOS と同じく使い捨ての UUID。 */
            fun of(c: Component, fallbackId: () -> String = { UUID.randomUUID().toString().uppercase() }): Config? {
                if (c.type != ComponentType.NEARBY_SHARE || c.mode != ComponentMode.PASSBY) return null
                val entity = c.entity ?: return null
                return Config(
                    componentId = c.id ?: fallbackId(), entityId = entity, label = c.label ?: "すれ違い通信", fields = c.fields,
                    limit = c.limit ?: DEFAULT_LIMIT, distance = c.distance ?: false,
                )
            }
        }
    }

    /** 送ってよい field(明示 ∩ 保存 field − hidden − 写真・動画・ref)。DSL の並び順。 */
    fun allowedFields(config: Config, entity: Entity): List<Field> {
        val listed = config.fields.toSet()
        return entity.storedFields.filter { it.id in listed && !it.hidden && it.type !in BLOCKED_TYPES }
    }

    // MARK: - 送受信データ

    data class Item(val id: String, val data: Map<String, JSONValue>) {
        fun toJson(): JsonObject = JsonObject(mapOf("id" to JsonPrimitive(id), "data" to NearbyJson.record(data)))

        companion object {
            fun from(e: JsonElement): Item {
                val o = NearbyJson.obj(e, "Item")
                return Item(NearbyJson.str(o, "id"), NearbyJson.record(o["data"], "data"))
            }
        }
    }

    /** 招待に載せる挨拶(小さく保つ)。`app` は appId のハッシュ(appId 自体は流さない)。 */
    data class Hello(val v: Int = VERSION, val app: String, val senderId: String, val name: String) {
        fun toJson(): JsonObject = JsonObject(
            mapOf("v" to JsonPrimitive(v), "app" to JsonPrimitive(app), "senderId" to JsonPrimitive(senderId), "name" to JsonPrimitive(name)),
        )

        companion object {
            fun from(e: JsonElement?): Hello {
                val o = NearbyJson.obj(e, "Hello")
                return Hello(NearbyJson.int(o, "v"), NearbyJson.str(o, "app"), NearbyJson.str(o, "senderId"), NearbyJson.str(o, "name"))
            }
        }
    }

    data class Payload(val v: Int, val senderId: String, val name: String, val entityId: String, val records: List<Item>) {
        fun toJson(): JsonObject = JsonObject(
            mapOf(
                "v" to JsonPrimitive(v),
                "senderId" to JsonPrimitive(senderId),
                "name" to JsonPrimitive(name),
                "entityId" to JsonPrimitive(entityId),
                "records" to JsonArray(records.map { it.toJson() }),
            ),
        )

        companion object {
            fun from(e: JsonElement?): Payload {
                val o = NearbyJson.obj(e, "Payload")
                return Payload(
                    v = NearbyJson.int(o, "v"),
                    senderId = NearbyJson.str(o, "senderId"),
                    name = NearbyJson.str(o, "name"),
                    entityId = NearbyJson.str(o, "entityId"),
                    records = NearbyJson.array(o, "records").map { Item.from(it) },
                )
            }
        }
    }

    /** 同じアプリかを比べる印(FNV-1a 64bit・小文字 16 進・暗号用途ではない)。iOS と同じ値になる。 */
    fun appTag(appId: String): String {
        var h = -0x340d631b7bdddcdbL // 0xcbf29ce484222325
        for (b in appId.toByteArray(Charsets.UTF_8)) {
            h = (h xor (b.toLong() and 0xFF)) * 0x100000001b3L
        }
        return java.lang.Long.toUnsignedString(h, 16)
    }

    /**
     * 送るデータ。自分の記録だけ(受け取った記録・他人の共有レコードは除く)を新しい順に `limit` 件、許可 field だけ。
     * 16 KB を超える分は古い方から落とす。
     */
    fun makePayload(
        senderId: String, name: String, entity: Entity, config: Config, records: List<Record>, ledger: Ledger, me: String?,
    ): Payload {
        val allowed = allowedFields(config, entity).map { it.id }.toSet()
        val items = mutableListOf<Item>()
        for (r in NearbyShare.newestFirst(records)) {
            if (items.size >= config.limit) break
            if (r.author != null && r.author != me) continue
            if (ledger.isReceived(r.data)) continue
            val kept = r.data.filter { it.key in allowed && !it.value.isNull }
            if (kept.isNotEmpty()) items.add(Item(r.id, kept))
        }
        while (items.isNotEmpty() && size(Payload(VERSION, senderId, name, entity.id, items)) > MAX_BYTES) items.removeAt(items.lastIndex)
        return Payload(VERSION, senderId, name, entity.id, items.toList())
    }

    internal fun size(p: Payload): Int = runCatching { encode(p).size }.getOrDefault(Int.MAX_VALUE)

    /** 受け取った記録を自分の部品の設定に揃える: 許可 field 以外・型の合わない値・null を捨て、空の行と超過分を除く。 */
    fun sanitize(payload: Payload, entity: Entity, config: Config): List<Item> {
        if (payload.entityId != entity.id) return emptyList()
        val fields = allowedFields(config, entity).reversed().associateBy { it.id }
        return payload.records.take(MAX_RECORDS).mapNotNull { item ->
            val data = item.data.filter { (k, v) -> fields[k]?.let { NearbyShare.accepts(v, it.type) } ?: false }
            if (data.isEmpty() || item.id.isEmpty()) null else Item(item.id, data)
        }
    }

    // MARK: - 会った記録と重複除け

    data class Encounter(
        /** 相手の senderId */
        val id: String,
        val name: String,
        val firstMet: Instant,
        val lastMet: Instant,
        /** 会った回数(cooldown より間が空いたら +1) */
        val times: Int,
        /** 受け取って追加・更新した記録の数(累計) */
        val records: Int,
    ) {
        fun toJson(): JsonObject = JsonObject(
            mapOf(
                "id" to JsonPrimitive(id), "name" to JsonPrimitive(name),
                "firstMet" to NearbyJson.date(firstMet), "lastMet" to NearbyJson.date(lastMet),
                "times" to JsonPrimitive(times), "records" to JsonPrimitive(records),
            ),
        )

        companion object {
            fun from(e: JsonElement): Encounter {
                val o = NearbyJson.obj(e, "Encounter")
                return Encounter(
                    NearbyJson.str(o, "id"), NearbyJson.str(o, "name"), NearbyJson.date(o, "firstMet"), NearbyJson.date(o, "lastMet"),
                    NearbyJson.int(o, "times"), NearbyJson.int(o, "records"),
                )
            }
        }
    }

    sealed class Plan {
        data class Create(val key: String, val data: Map<String, JSONValue>) : Plan()

        /** 前回受け取った内容(`old`)の記録を `new` に更新する */
        data class Update(val key: String, val old: Map<String, JSONValue>, val new: Map<String, JSONValue>) : Plan()
        data class Skip(val key: String) : Plan()
    }

    /** 端末に保存する台帳(アプリごと)。受け取った記録の内容と、会った人の一覧。 */
    class Ledger(
        /** "senderId|recordId" → 最後に保存した内容 */
        received: Map<String, Map<String, JSONValue>> = emptyMap(),
        encounters: List<Encounter> = emptyList(),
    ) {
        private val _received = LinkedHashMap(received)
        private val _encounters = encounters.toMutableList()
        val received: Map<String, Map<String, JSONValue>> get() = _received
        val encounters: List<Encounter> get() = _encounters

        /** 受け取った記録か(保存した内容がそのまま残っている)。送り返さないための判定。 */
        fun isReceived(data: Map<String, JSONValue>): Boolean = _received.values.any { matches(data, it) }

        fun plan(items: List<Item>, sender: String): List<Plan> = items.map { item ->
            val k = key(sender, item.id)
            val old = _received[k] ?: return@map Plan.Create(k, item.data)
            if (old == item.data) Plan.Skip(k) else Plan.Update(k, old, item.data)
        }

        fun commit(key: String, data: Map<String, JSONValue>) {
            _received[key] = data
            if (_received.size > MAX_RECEIVED) _received.keys.minWithOrNull(NearbyJson.CodePointOrder)?.let { _received.remove(it) }
        }

        /** 交換してよいか(同じ相手とは cooldown に 1 回)。 */
        fun shouldExchange(sender: String, now: Instant): Boolean {
            val e = _encounters.firstOrNull { it.id == sender } ?: return true
            return Duration.between(e.lastMet, now) >= COOLDOWN
        }

        /** 会った記録を付ける。新しい順に並べ、上限を超えた古い人は消す。 */
        fun met(sender: String, name: String, now: Instant, added: Int) {
            val i = _encounters.indexOfFirst { it.id == sender }
            if (i >= 0) {
                val e = _encounters.removeAt(i)
                val times = if (Duration.between(e.lastMet, now) >= COOLDOWN) e.times + 1 else e.times
                _encounters.add(0, e.copy(name = name, lastMet = now, times = times, records = e.records + added))
            } else {
                _encounters.add(0, Encounter(sender, name, now, now, 1, added))
            }
            while (_encounters.size > MAX_ENCOUNTERS) _encounters.removeAt(_encounters.lastIndex)
        }

        fun toJson(): JsonObject = JsonObject(
            mapOf(
                "received" to JsonObject(_received.mapValues { NearbyJson.record(it.value) }),
                "encounters" to JsonArray(_encounters.map { it.toJson() }),
            ),
        )

        override fun equals(other: Any?): Boolean = other is Ledger && received == other.received && encounters == other.encounters
        override fun hashCode(): Int = 31 * received.hashCode() + encounters.hashCode()

        companion object {
            const val MAX_ENCOUNTERS = 200
            const val MAX_RECEIVED = 1000

            fun key(sender: String, id: String): String = "$sender|$id"

            /** `record` が前回保存した `saved` の内容を全部持っているか。 */
            fun matches(record: Map<String, JSONValue>, saved: Map<String, JSONValue>): Boolean =
                saved.isNotEmpty() && saved.all { record[it.key] == it.value }

            /** 前回保存した内容の記録を探す(ローカルの仮 id はサーバー保存で変わるので内容で探す)。 */
            fun match(saved: Map<String, JSONValue>, records: List<Record>): Record? = records.firstOrNull { matches(it.data, saved) }

            fun from(e: JsonElement?): Ledger {
                val o = NearbyJson.obj(e, "Ledger")
                val received = NearbyJson.obj(o["received"], "received").mapValues { NearbyJson.record(it.value, "received") }
                return Ledger(received, NearbyJson.array(o, "encounters").map { Encounter.from(it) })
            }
        }
    }

    // MARK: - 同意とスイッチ

    enum class ToggleOutcome {
        /** 初回: 何を誰に送るかの同意を先に出す */
        ASK_CONSENT,
        START,
        STOP,
    }

    fun toggle(on: Boolean, consentedVersion: Int?): ToggleOutcome {
        if (!on) return ToggleOutcome.STOP
        return if ((consentedVersion ?: 0) >= CONSENT_VERSION) ToggleOutcome.START else ToggleOutcome.ASK_CONSENT
    }

    /** 同意文。送る項目の名前を必ず並べる。 */
    fun consentText(fieldNames: List<String>): String {
        val list = if (fieldNames.isEmpty()) "(送る項目がありません)" else fieldNames.joinToString("") { "「$it」" }
        return "オンの間、近くで同じアプリを開いている人と自動でつながり、あなたの${list}を交換します。" +
            "相手の記録はこの端末に追加されます。写真・動画や、それ以外の項目は送りません。" +
            "この画面を閉じるか、アプリを裏に回すと止まります。"
    }

    // MARK: - 距離・方向の表示

    fun distanceText(meters: Float): String {
        if (meters < 10) return String.format(Locale.ROOT, "%.1f m", meters.toDouble())
        val m = meters.toDouble()
        return "${(sign(m) * floor(abs(m) + 0.5)).toInt()} m" // Swift の rounded(): 0.5 は 0 から遠い方へ
    }

    /** 水平方向の角度(ラジアン・右が正)を言葉に。 */
    fun directionText(azimuth: Float): String {
        val deg = azimuth * 180 / Math.PI.toFloat()
        return when {
            deg >= -20f && deg <= 20f -> "正面"
            deg >= 20f && deg <= 70f -> "右前"
            deg >= -70f && deg <= -20f -> "左前"
            deg >= 70f -> "右"
            else -> "左"
        }
    }

    // MARK: - 符号化

    fun encode(payload: Payload): ByteArray = NearbyJson.bytes(payload.toJson())
    fun encode(hello: Hello): ByteArray = NearbyJson.bytes(hello.toJson())
    fun encode(ledger: Ledger): ByteArray = NearbyJson.bytes(ledger.toJson())

    fun decodePayload(data: ByteArray): Payload = Payload.from(NearbyJson.parse(data))
    fun decodeHello(data: ByteArray): Hello = Hello.from(NearbyJson.parse(data))
    fun decodeLedger(data: ByteArray): Ledger = Ledger.from(NearbyJson.parse(data))
}
