package jp.kz2601.metamon.desktop.hotkey

import java.io.ByteArrayOutputStream
import java.nio.ByteBuffer
import java.nio.ByteOrder

// GlobalShortcuts のポータル(#2203)に要るだけの D-Bus の読み書き(ライブラリを足さない)。
// 型の対応: y→Int, b→Boolean, n/q/i→Int, u/x/t→Long, d→Double, s/g→String, o→DBusObjectPath, v→DBusVariant,
// a{..}→Map, a..→List, (..)→List。

internal data class DBusObjectPath(val path: String)

internal data class DBusVariant(val signature: String, val value: Any?)

/** 署名の読み方。 */
internal object DBusSignature {
    /** [sig] の [start] から 1 つの完全な型の終わり(含まない)。 */
    fun end(sig: String, start: Int): Int = when (val c = sig[start]) {
        'a' -> end(sig, start + 1)
        '(', '{' -> {
            val close = if (c == '(') ')' else '}'
            var i = start + 1
            while (sig[i] != close) i = end(sig, i)
            i + 1
        }
        else -> start + 1
    }

    /** 完全な型ごとに分ける(`sa{sv}` → `s`, `a{sv}`)。 */
    fun split(sig: String): List<String> {
        val out = mutableListOf<String>()
        var i = 0
        while (i < sig.length) {
            val e = end(sig, i)
            out += sig.substring(i, e)
            i = e
        }
        return out
    }

    fun alignment(type: Char): Int = when (type) {
        'y', 'g', 'v' -> 1
        'n', 'q' -> 2
        'b', 'i', 'u', 's', 'o', 'a' -> 4
        'x', 't', 'd', '(', '{' -> 8
        else -> throw IllegalArgumentException("unknown D-Bus type $type")
    }
}

/** 書く(リトルエンディアン)。位置は先頭からのバイト数(揃えはメッセージの先頭から数える)。 */
internal class DBusWriter {
    private val out = ByteArrayOutputStream()
    val size: Int get() = out.size()

    fun bytes(): ByteArray = out.toByteArray()

    fun align(n: Int) {
        while (out.size() % n != 0) out.write(0)
    }

    fun byte(v: Int) = out.write(v and 0xFF)

    fun uint32(v: Long) {
        align(4)
        out.write(ByteBuffer.allocate(4).order(ByteOrder.LITTLE_ENDIAN).putInt(v.toInt()).array())
    }

    private fun raw(b: ByteArray) = out.write(b)

    /** [sig] は 1 つの完全な型。 */
    fun write(sig: String, value: Any?) {
        when (val c = sig[0]) {
            'y' -> byte((value as Number).toInt())
            'b' -> uint32(if (value as Boolean) 1 else 0)
            'n', 'q' -> {
                align(2)
                raw(ByteBuffer.allocate(2).order(ByteOrder.LITTLE_ENDIAN).putShort((value as Number).toShort()).array())
            }
            'i', 'u' -> uint32((value as Number).toLong())
            'x', 't' -> {
                align(8)
                raw(ByteBuffer.allocate(8).order(ByteOrder.LITTLE_ENDIAN).putLong((value as Number).toLong()).array())
            }
            'd' -> {
                align(8)
                raw(ByteBuffer.allocate(8).order(ByteOrder.LITTLE_ENDIAN).putDouble((value as Number).toDouble()).array())
            }
            's', 'o' -> {
                val s = if (value is DBusObjectPath) value.path else value as String
                val b = s.toByteArray(Charsets.UTF_8)
                uint32(b.size.toLong())
                raw(b)
                byte(0)
            }
            'g' -> {
                val b = (value as String).toByteArray(Charsets.US_ASCII)
                byte(b.size)
                raw(b)
                byte(0)
            }
            'v' -> {
                val v = value as DBusVariant
                write("g", v.signature)
                write(v.signature, v.value)
            }
            'a' -> writeArray(sig.substring(1), value)
            '(' -> {
                align(8)
                val items = value as List<*>
                DBusSignature.split(sig.substring(1, sig.length - 1)).forEachIndexed { i, t -> write(t, items[i]) }
            }
            else -> throw IllegalArgumentException("unknown D-Bus type $c")
        }
    }

    private fun writeArray(element: String, value: Any?) {
        uint32(0)
        val lengthAt = out.size() - 4
        align(DBusSignature.alignment(element[0]))
        val start = out.size()
        if (element[0] == '{') {
            val (k, v) = DBusSignature.split(element.substring(1, element.length - 1))
            (value as Map<*, *>).forEach { (key, item) ->
                align(8)
                write(k, key)
                write(v, item)
            }
        } else {
            (value as List<*>).forEach { write(element, it) }
        }
        patchUint32(lengthAt, (out.size() - start).toLong())
    }

    private fun patchUint32(at: Int, v: Long) {
        val all = out.toByteArray()
        ByteBuffer.wrap(all).order(ByteOrder.LITTLE_ENDIAN).putInt(at, v.toInt())
        out.reset()
        out.write(all)
    }
}

/** 読む。[order] は先頭の `l` / `B` から。 */
internal class DBusReader(private val data: ByteArray, private val order: ByteOrder, start: Int = 0) {
    var pos = start
        private set

    private fun buf(n: Int): ByteBuffer {
        if (pos + n > data.size) throw IllegalArgumentException("D-Bus message truncated")
        return ByteBuffer.wrap(data, pos, n).order(order).also { pos += n }
    }

    fun align(n: Int) {
        while (pos % n != 0) pos++
    }

    fun read(sig: String): Any? = when (val c = sig[0]) {
        'y' -> buf(1).get().toInt() and 0xFF
        'b' -> {
            align(4)
            buf(4).int != 0
        }
        'n' -> {
            align(2)
            buf(2).short.toInt()
        }
        'q' -> {
            align(2)
            buf(2).short.toInt() and 0xFFFF
        }
        'i' -> {
            align(4)
            buf(4).int
        }
        'u' -> {
            align(4)
            buf(4).int.toLong() and 0xFFFFFFFFL
        }
        'x', 't' -> {
            align(8)
            buf(8).long
        }
        'd' -> {
            align(8)
            buf(8).double
        }
        's', 'o' -> {
            val n = (read("u") as Long).toInt()
            val s = String(data, pos, n, Charsets.UTF_8)
            pos += n + 1
            if (c == 'o') DBusObjectPath(s) else s
        }
        'g' -> {
            val n = read("y") as Int
            val s = String(data, pos, n, Charsets.US_ASCII)
            pos += n + 1
            s
        }
        'v' -> {
            val s = read("g") as String
            DBusVariant(s, read(s))
        }
        'a' -> readArray(sig.substring(1))
        '(' -> {
            align(8)
            DBusSignature.split(sig.substring(1, sig.length - 1)).map { read(it) }
        }
        else -> throw IllegalArgumentException("unknown D-Bus type $c")
    }

    private fun readArray(element: String): Any {
        val len = (read("u") as Long).toInt()
        align(DBusSignature.alignment(element[0]))
        val end = pos + len
        return if (element[0] == '{') {
            val (k, v) = DBusSignature.split(element.substring(1, element.length - 1))
            val map = LinkedHashMap<Any?, Any?>()
            while (pos < end) {
                align(8)
                val key = read(k)
                map[key] = read(v)
            }
            map
        } else {
            val list = mutableListOf<Any?>()
            while (pos < end) list += read(element)
            list
        }
    }

    fun readAll(signature: String): List<Any?> = DBusSignature.split(signature).map { read(it) }
}

/** D-Bus のメッセージ 1 つ。 */
internal data class DBusMessage(
    val type: Int,
    val serial: Long = 0,
    val path: String? = null,
    val iface: String? = null,
    val member: String? = null,
    val errorName: String? = null,
    val replySerial: Long? = null,
    val destination: String? = null,
    val sender: String? = null,
    val signature: String = "",
    val body: List<Any?> = emptyList(),
) {
    companion object {
        const val METHOD_CALL = 1
        const val METHOD_RETURN = 2
        const val ERROR = 3
        const val SIGNAL = 4

        private const val F_PATH = 1
        private const val F_INTERFACE = 2
        private const val F_MEMBER = 3
        private const val F_ERROR_NAME = 4
        private const val F_REPLY_SERIAL = 5
        private const val F_DESTINATION = 6
        private const val F_SENDER = 7
        private const val F_SIGNATURE = 8

        /** 先頭 16 バイトから、メッセージ全体の長さ。 */
        fun totalLength(head: ByteArray): Int {
            val order = orderOf(head[0])
            val b = ByteBuffer.wrap(head).order(order)
            val bodyLen = b.getInt(4)
            val fieldsLen = b.getInt(12)
            val headerEnd = 16 + fieldsLen
            return headerEnd + (8 - headerEnd % 8) % 8 + bodyLen
        }

        private fun orderOf(b: Byte): ByteOrder = when (b.toInt().toChar()) {
            'l' -> ByteOrder.LITTLE_ENDIAN
            'B' -> ByteOrder.BIG_ENDIAN
            else -> throw IllegalArgumentException("bad D-Bus endianness")
        }

        fun decode(data: ByteArray): DBusMessage {
            val r = DBusReader(data, orderOf(data[0]))
            r.read("y")
            val type = r.read("y") as Int
            r.read("y")
            r.read("y")
            r.read("u")
            val serial = r.read("u") as Long
            @Suppress("UNCHECKED_CAST")
            val fields = (r.read("a(yv)") as List<List<Any?>>).associate { (code, v) -> (code as Int) to (v as DBusVariant).value }
            r.align(8)
            val signature = fields[F_SIGNATURE] as? String ?: ""
            val body = if (signature.isEmpty()) emptyList() else r.readAll(signature)
            return DBusMessage(
                type = type,
                serial = serial,
                path = (fields[F_PATH] as? DBusObjectPath)?.path,
                iface = fields[F_INTERFACE] as? String,
                member = fields[F_MEMBER] as? String,
                errorName = fields[F_ERROR_NAME] as? String,
                replySerial = fields[F_REPLY_SERIAL] as? Long,
                destination = fields[F_DESTINATION] as? String,
                sender = fields[F_SENDER] as? String,
                signature = signature,
                body = body,
            )
        }
    }

    fun encode(): ByteArray {
        val bodyW = DBusWriter()
        DBusSignature.split(signature).forEachIndexed { i, t -> bodyW.write(t, body[i]) }
        val body = bodyW.bytes()
        val fields = buildList<List<Any?>> {
            path?.let { add(listOf(F_PATH, DBusVariant("o", DBusObjectPath(it)))) }
            iface?.let { add(listOf(F_INTERFACE, DBusVariant("s", it))) }
            member?.let { add(listOf(F_MEMBER, DBusVariant("s", it))) }
            errorName?.let { add(listOf(F_ERROR_NAME, DBusVariant("s", it))) }
            replySerial?.let { add(listOf(F_REPLY_SERIAL, DBusVariant("u", it))) }
            destination?.let { add(listOf(F_DESTINATION, DBusVariant("s", it))) }
            sender?.let { add(listOf(F_SENDER, DBusVariant("s", it))) }
            if (signature.isNotEmpty()) add(listOf(F_SIGNATURE, DBusVariant("g", signature)))
        }
        val w = DBusWriter()
        w.byte('l'.code)
        w.byte(type)
        w.byte(0)
        w.byte(1)
        w.uint32(body.size.toLong())
        w.uint32(serial)
        w.write("a(yv)", fields)
        w.align(8)
        return w.bytes() + body
    }
}

/** `$DBUS_SESSION_BUS_ADDRESS` の読み方。 */
internal object DBusAddress {
    /** `unix:path=…` の最初の 1 つ(`%xx` を戻す)。abstract・tcp などは null(Java のソケットで開けない)。 */
    fun unixPath(address: String?): String? {
        address ?: return null
        for (entry in address.split(';')) {
            if (!entry.startsWith("unix:")) continue
            val kv = entry.removePrefix("unix:").split(',').mapNotNull { p -> p.split('=', limit = 2).takeIf { it.size == 2 }?.let { it[0] to it[1] } }.toMap()
            kv["path"]?.let { return unescape(it) }
        }
        return null
    }

    private fun unescape(s: String): String {
        val out = ByteArrayOutputStream()
        var i = 0
        while (i < s.length) {
            if (s[i] == '%' && i + 2 < s.length) {
                out.write(s.substring(i + 1, i + 3).toInt(16))
                i += 3
            } else {
                out.write(s[i].code)
                i++
            }
        }
        return out.toString(Charsets.UTF_8)
    }

    /** AUTH EXTERNAL に渡す uid(10 進の文字の 16 進)。 */
    fun externalAuth(uid: Long): String = uid.toString().toByteArray(Charsets.US_ASCII).joinToString("") { "%02x".format(it) }
}
