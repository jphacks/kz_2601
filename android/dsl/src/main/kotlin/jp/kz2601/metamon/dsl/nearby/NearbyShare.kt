package jp.kz2601.metamon.dsl.nearby

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.dsl.records.entityModels
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive

/**
 * `nearbyShare`(#36)の送受信データと取り込み判定。通信はアプリ本体、ここは純粋ロジックだけ。iOS の `NearbyShare`。
 *
 * - 招待時: [Header] を載せる(受信側は接続前に「誰から・何が・何件」と取り込めるかを判定できる)
 * - 接続後: [Payload](Header + レコード)を JSON で 1 回送り、受信側は [Ack] を返す
 * - 未知のキーは読み飛ばし、レコード内の未知フィールドは [sanitize] で捨てる
 */
object NearbyShare {
    const val VERSION = 1

    /** 一覧画面から送る最大件数(新しい順)。 */
    const val MAX_SEND_RECORDS = 50

    /** 受信側が取り込む最大件数。 */
    const val MAX_RECEIVE_RECORDS = 200

    /** 曲のリクエストの [Decision.Accept] に入れる印(entity ではない)。 */
    const val SONG_ENTITY_ID = "\$musicRequest"

    data class FieldSignature(val id: String, val type: String) {
        fun toJson(): JsonObject = JsonObject(mapOf("id" to JsonPrimitive(id), "type" to JsonPrimitive(type)))

        companion object {
            fun from(e: JsonElement): FieldSignature {
                val o = NearbyJson.obj(e, "FieldSignature")
                return FieldSignature(NearbyJson.str(o, "id"), NearbyJson.str(o, "type"))
            }
        }
    }

    /** 招待に載せる要約(小さく保つ)。`song` = 曲のリクエスト(#412)・null = レコードの共有。 */
    data class Header(
        val v: Int = VERSION,
        val sender: String,
        val appId: String,
        val appName: String,
        val entityId: String,
        val entityName: String,
        val fields: List<FieldSignature>,
        val count: Int,
        val song: MusicSong? = null,
    ) {
        fun toJson(): JsonObject = JsonObject(
            buildMap {
                put("v", JsonPrimitive(v))
                put("sender", JsonPrimitive(sender))
                put("appId", JsonPrimitive(appId))
                put("appName", JsonPrimitive(appName))
                put("entityId", JsonPrimitive(entityId))
                put("entityName", JsonPrimitive(entityName))
                put("fields", JsonArray(fields.map { it.toJson() }))
                put("count", JsonPrimitive(count))
                song?.let { put("song", it.toJson()) }
            },
        )

        companion object {
            fun of(sender: String, appId: String, appName: String, entity: Entity, count: Int) = Header(
                sender = sender, appId = appId, appName = appName, entityId = entity.id, entityName = entity.name,
                fields = signature(entity), count = count,
            )

            /** 曲のリクエスト 1 件(レコードは送らない)。 */
            fun song(sender: String, appId: String, appName: String, song: MusicSong) = Header(
                sender = sender, appId = appId, appName = appName, entityId = SONG_ENTITY_ID, entityName = "曲のリクエスト",
                fields = emptyList(), count = 1, song = song,
            )

            fun from(e: JsonElement?): Header {
                val o = NearbyJson.obj(e, "Header")
                return Header(
                    v = NearbyJson.int(o, "v"),
                    sender = NearbyJson.str(o, "sender"),
                    appId = NearbyJson.str(o, "appId"),
                    appName = NearbyJson.str(o, "appName"),
                    entityId = NearbyJson.str(o, "entityId"),
                    entityName = NearbyJson.str(o, "entityName"),
                    fields = NearbyJson.array(o, "fields").map { FieldSignature.from(it) },
                    count = NearbyJson.int(o, "count"),
                    song = o["song"]?.takeIf { it !is JsonNull }?.let { MusicSong.from(it) },
                )
            }
        }
    }

    data class Payload(val header: Header, val records: List<Map<String, JSONValue>>) {
        fun toJson(): JsonObject = JsonObject(
            mapOf("header" to header.toJson(), "records" to JsonArray(records.map { NearbyJson.record(it) })),
        )

        companion object {
            fun from(e: JsonElement?): Payload {
                val o = NearbyJson.obj(e, "Payload")
                return Payload(
                    header = Header.from(o["header"]),
                    records = NearbyJson.array(o, "records").map { NearbyJson.record(it, "record") },
                )
            }
        }
    }

    /** 受信側 → 送信側の結果。 */
    data class Ack(val ok: Boolean, val added: Int, val message: String? = null) {
        fun toJson(): JsonObject = JsonObject(
            buildMap {
                put("ok", JsonPrimitive(ok))
                put("added", JsonPrimitive(added))
                message?.let { put("message", JsonPrimitive(it)) }
            },
        )

        companion object {
            fun from(e: JsonElement?): Ack {
                val o = NearbyJson.obj(e, "Ack")
                return Ack(NearbyJson.bool(o, "ok"), NearbyJson.int(o, "added"), NearbyJson.optStr(o, "message"))
            }
        }
    }

    /** 取り込み判定の結果。 */
    sealed class Decision {
        /** 取り込める。`sameApp` = appId 一致(ref フィールドもそのまま使える)。 */
        data class Accept(val entityId: String, val sameApp: Boolean) : Decision()
        data class Reject(val reason: String) : Decision()
    }

    // MARK: - 送信

    fun signature(entity: Entity): List<FieldSignature> = entity.storedFields.map { FieldSignature(it.id, it.type.wire) }

    /** 送るレコードを作る。保存フィールド以外(computed・未知キー)と null は落とし、空になったものは送らない。 */
    fun makePayload(sender: String, appId: String, appName: String, entity: Entity, records: List<Map<String, JSONValue>>): Payload {
        val stored = entity.storedFields.map { it.id }.toSet()
        val rows = records.take(MAX_SEND_RECORDS).mapNotNull { data ->
            data.filter { it.key in stored && !it.value.isNull }.takeIf { it.isNotEmpty() }
        }
        return Payload(Header.of(sender, appId, appName, entity, rows.size), rows)
    }

    /** 一覧画面で送る範囲: 新しい順(createdAt 降順・無ければ元の順)に最大 [MAX_SEND_RECORDS] 件。 */
    fun recordsToSend(records: List<Record>): List<Map<String, JSONValue>> =
        newestFirst(records).take(MAX_SEND_RECORDS).map { it.data }

    /** createdAt 降順(同じなら元の順)。iOS と同じく文字列で比べる。 */
    internal fun newestFirst(records: List<Record>): List<Record> =
        records.withIndex().sortedWith { a, b ->
            val ka = a.value.createdAt ?: ""
            val kb = b.value.createdAt ?: ""
            if (ka == kb) a.index.compareTo(b.index) else NearbyJson.CodePointOrder.compare(kb, ka)
        }.map { it.value }

    // MARK: - 受信

    /** 受け取れるか。同じ appId なら entity id が在れば OK。別アプリなら entity id と保存フィールド(id+型)の集合が一致する時だけ。 */
    fun decide(header: Header, dsl: AppDSL, appId: String): Decision =
        decide(header, dsl.entityModels(), dsl.usesMusicRequests, appId)

    fun decide(header: Header, entities: List<Entity>, usesMusicRequests: Boolean, appId: String): Decision {
        if (header.v > VERSION) return Decision.Reject("相手のアプリが新しすぎるため受け取れません")
        header.song?.let { return decideSong(it, usesMusicRequests, sameApp = header.appId == appId) }
        val entity = entities.firstOrNull { it.id == header.entityId }
            ?: return Decision.Reject("このアプリには「${header.entityName}」がないため受け取れません")
        if (header.appId == appId) return Decision.Accept(entity.id, sameApp = true)
        if (signature(entity).toSet() != header.fields.toSet()) {
            return Decision.Reject("「${header.entityName}」の項目が違うため受け取れません(同じアプリを開いてください)")
        }
        return Decision.Accept(entity.id, sameApp = false)
    }

    /**
     * 受け取ったレコードを自分の entity の形に揃える: 未知フィールド・型が合わない値・null を捨てる。
     * 別アプリからの ref は相手端末のレコード id なので捨てる。空になった行は除く。
     */
    fun sanitize(records: List<Map<String, JSONValue>>, entity: Entity, sameApp: Boolean): List<Map<String, JSONValue>> {
        val fields = entity.storedFields.reversed().associateBy { it.id } // 同じ id は先の方を使う
        return records.take(MAX_RECEIVE_RECORDS).mapNotNull { data ->
            val out = data.filter { (key, value) ->
                val f = fields[key] ?: return@filter false
                accepts(value, f.type) && !((f.type == FieldType.REF || f.type == FieldType.REFS) && !sameApp)
            }
            out.takeIf { it.isNotEmpty() }
        }
    }

    /** 値が field の型に合うか(null・member などは受けない)。 */
    fun accepts(value: JSONValue, type: FieldType): Boolean = when (type) {
        FieldType.INTEGER, FieldType.NUMBER, FieldType.CURRENCY -> value is JSONValue.Num
        FieldType.BOOLEAN -> value is JSONValue.Bool
        FieldType.STRING, FieldType.TEXT, FieldType.DATE, FieldType.DATETIME, FieldType.ENUM, FieldType.PHOTO,
        FieldType.VIDEO, FieldType.AUDIO, FieldType.LOCATION, FieldType.REF,
        -> value is JSONValue.Str
        FieldType.REFS -> value is JSONValue.Arr
        FieldType.MEMBER, FieldType.MEMBERS -> false
    }

    // MARK: - 曲のリクエスト(#412)

    /** 送る曲のリクエストを作る。曲名が無ければ null。 */
    fun makeSongRequest(song: MusicSong, sender: String, appId: String, appName: String): Payload? {
        val s = MusicRequestRules.sanitize(song) ?: return null
        return Payload(Header.song(sender, appId, appName, s), emptyList())
    }

    /** 今のアプリが allowRequests の musicPlayer を使っている時だけ受け取る(同じアプリの別コピーでもよい)。 */
    internal fun decideSong(song: MusicSong, usesMusicRequests: Boolean, sameApp: Boolean): Decision {
        if (!usesMusicRequests) return Decision.Reject("このアプリは曲のリクエストを受け付けていません")
        if (MusicRequestRules.sanitize(song) == null) return Decision.Reject("曲名のないリクエストは受け取れません")
        return Decision.Accept(SONG_ENTITY_ID, sameApp)
    }

    // MARK: - 符号化

    fun encode(payload: Payload): ByteArray = NearbyJson.bytes(payload.toJson())
    fun encode(header: Header): ByteArray = NearbyJson.bytes(header.toJson())
    fun encode(ack: Ack): ByteArray = NearbyJson.bytes(ack.toJson())

    fun decodePayload(data: ByteArray): Payload = Payload.from(NearbyJson.parse(data))
    fun decodeHeader(data: ByteArray): Header = Header.from(NearbyJson.parse(data))
    fun decodeAck(data: ByteArray): Ack = Ack.from(NearbyJson.parse(data))
}
