package jp.kz2601.metamon.nearby

import jp.kz2601.metamon.dsl.nearby.NearbyShare
import java.io.ByteArrayOutputStream

/**
 * 近距離共有の 1 通を Nearby Connections の BYTES(1 回 32 KB まで)に分けて送る形。
 * iOS は招待の context に Header を載せるが、Nearby Connections の申し出には載らないので、つないだ後に順に送る:
 * 送る側 HEADER → 受ける側 READY(承認)→ 送る側 PAYLOAD → 受ける側 ACK。中身の JSON は iOS と同じ([NearbyShare])。
 *
 * 1 切れ = [種類 1 バイト][最後か 1 バイト][中身]。受ける側は種類ごとの上限を超えたら読むのをやめる。
 */
object NearbyWire {
    /** Nearby Connections の BYTES の上限(ConnectionsClient.MAX_BYTES_DATA_SIZE)。 */
    const val MAX_CHUNK_BYTES = 32 * 1024
    private const val HEAD = 2
    private const val BODY = MAX_CHUNK_BYTES - HEAD

    enum class Kind(val code: Byte, val maxBytes: Int) {
        HEADER('H'.code.toByte(), NearbyShare.MAX_HEADER_BYTES),
        READY('R'.code.toByte(), 0),
        PAYLOAD('P'.code.toByte(), NearbyShare.MAX_RECEIVE_BYTES),
        ACK('A'.code.toByte(), NearbyShare.MAX_ACK_BYTES),
        ;

        companion object {
            fun of(code: Byte): Kind? = entries.firstOrNull { it.code == code }
        }
    }

    /** 1 通を切れに分ける(空の中身でも 1 切れ)。 */
    fun frames(kind: Kind, body: ByteArray): List<ByteArray> {
        val parts = if (body.isEmpty()) listOf(body) else (body.indices step BODY).map { body.copyOfRange(it, minOf(it + BODY, body.size)) }
        return parts.mapIndexed { i, part ->
            ByteArray(HEAD + part.size).also {
                it[0] = kind.code
                it[1] = if (i == parts.lastIndex) 1 else 0
                part.copyInto(it, HEAD)
            }
        }
    }

    data class Message(val kind: Kind, val body: ByteArray) {
        override fun equals(other: Any?): Boolean = other is Message && kind == other.kind && body.contentEquals(other.body)
        override fun hashCode(): Int = 31 * kind.hashCode() + body.contentHashCode()
    }

    /** 届いた切れを 1 通にまとめる(相手 1 人に 1 つ)。上限を超えた・形の違う切れが来たら、その相手からは読まない。 */
    class Reader {
        private var kind: Kind? = null
        private val buffer = ByteArrayOutputStream()

        /** 壊れた・大きすぎるものが来た(以後は読まない)。 */
        var broken = false
            private set

        /** 1 通そろったら返す。 */
        fun accept(frame: ByteArray): Message? {
            if (broken) return null
            val k = frame.takeIf { it.size >= HEAD }?.let { Kind.of(it[0]) }
            if (k == null || (kind != null && kind != k)) return fail()
            kind = k
            if (buffer.size() + frame.size - HEAD > k.maxBytes) return fail()
            buffer.write(frame, HEAD, frame.size - HEAD)
            if (frame[1].toInt() != 1) return null
            val body = buffer.toByteArray()
            buffer.reset()
            kind = null
            return Message(k, body)
        }

        private fun fail(): Message? {
            broken = true
            buffer.reset()
            kind = null
            return null
        }
    }
}
