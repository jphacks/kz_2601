package jp.kz2601.metamon.desktop.hotkey

import com.sun.jna.Library
import com.sun.jna.Native
import jp.kz2601.metamon.desktop.i18n.Strings
import java.io.InputStream
import java.io.OutputStream
import java.net.StandardProtocolFamily
import java.net.UnixDomainSocketAddress
import java.nio.channels.Channels
import java.nio.channels.SocketChannel
import java.util.concurrent.CompletableFuture
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.CopyOnWriteArrayList
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicLong

/** ポータルの Request の Response 信号。code: 0 = できた・1 = 人が取り消した・2 = その他。 */
internal data class PortalResponse(val code: Long, val results: Map<String, Any?> = emptyMap()) {
    companion object {
        const val SUCCESS = 0L
        const val CANCELLED = 1L
        const val OTHER = 2L
    }
}

/** xdg-desktop-portal の `org.freedesktop.portal.GlobalShortcuts`(テストで作り物に替える)。 */
internal interface ShortcutsPortal : AutoCloseable {
    /** GlobalShortcuts の版。ポータルに GlobalShortcuts が無ければ null。 */
    fun version(): Int?

    /** CreateSession。できれば results の `session_handle` に session の path。 */
    fun createSession(): PortalResponse

    /** BindShortcuts。キーの組み 1 つ([trigger] は希望。決めるのはデスクトップ環境)。 */
    fun bind(session: String, id: String, description: String, trigger: String?): PortalResponse

    /** Activated の信号(session の path・キーの id)。 */
    fun onActivated(listener: (session: String, id: String) -> Unit)

    fun closeSession(session: String)
}

/** ポータルに渡す希望のキー(XDG の shortcuts の書き方: `CTRL+SHIFT+space`)。 */
internal object PortalTrigger {
    fun of(c: HotkeyChord): String? {
        val key = when (val vk = c.vk) {
            0x20 -> "space"
            0x0D -> "Return"
            0x09 -> "Tab"
            0x1B -> "Escape"
            0x2D -> "Insert"
            0x24 -> "Home"
            0x23 -> "End"
            0x13 -> "Pause"
            in 0x41..0x5A -> ('a' + (vk - 0x41)).toString()
            in 0x30..0x39 -> ('0' + (vk - 0x30)).toString()
            in 0x70..0x87 -> "F${vk - 0x70 + 1}"
            else -> return null
        }
        return (listOfNotNull("CTRL".takeIf { c.ctrl }, "ALT".takeIf { c.alt }, "SHIFT".takeIf { c.shift }, "LOGO".takeIf { c.meta }) + key).joinToString("+")
    }
}

/**
 * Linux の Wayland(#2203): アプリはキーを勝手に取れないので、ポータルの GlobalShortcuts に頼む。
 * `CreateSession` → `BindShortcuts`(デスクトップ環境が確認を出すことがある)→ `Activated` の信号で [onPressed]。
 * ポータルが無い・GlobalShortcuts が無い時は [HotkeyFailure.UNSUPPORTED](理由は [HotkeyAvailability])。
 */
class PortalGlobalHotkey internal constructor(
    private val connect: () -> ShortcutsPortal?,
    private val description: () -> String = { "Talk to Tailor It" },
) : GlobalHotkey {
    constructor() : this({ DBusShortcutsPortal.connect() }, { Strings["desktop_hotkey_title"] })

    private var portal: ShortcutsPortal? = null
    private var session: String? = null

    @Synchronized
    override fun register(chord: HotkeyChord, onPressed: () -> Unit): HotkeyRegistration {
        unregister()
        val trigger = PortalTrigger.of(chord) ?: return HotkeyRegistration.Failed(HotkeyFailure.ERROR, "no trigger for ${chord.keyName}")
        val p = runCatching(connect).getOrNull() ?: return HotkeyRegistration.Failed(HotkeyFailure.UNSUPPORTED, NO_PORTAL)
        val result = runCatching { bind(p, chord, trigger, onPressed) }.getOrElse { HotkeyRegistration.Failed(HotkeyFailure.ERROR, it.message) }
        if (result !is HotkeyRegistration.Registered) {
            session?.let { s -> runCatching { p.closeSession(s) } }
            session = null
            runCatching { p.close() }
        } else {
            portal = p
        }
        return result
    }

    private fun bind(p: ShortcutsPortal, chord: HotkeyChord, trigger: String, onPressed: () -> Unit): HotkeyRegistration {
        p.version() ?: return HotkeyRegistration.Failed(HotkeyFailure.UNSUPPORTED, NO_GLOBAL_SHORTCUTS)
        val created = p.createSession()
        val s = sessionHandle(created) ?: return failure(created)
        session = s
        p.onActivated { activated, id -> if (activated == s && id == SHORTCUT_ID) runCatching(onPressed) }
        val bound = p.bind(s, SHORTCUT_ID, description(), trigger)
        return if (bound.code == PortalResponse.SUCCESS) HotkeyRegistration.Registered(chord) else failure(bound)
    }

    @Synchronized
    override fun unregister() {
        val p = portal ?: return
        portal = null
        session?.let { runCatching { p.closeSession(it) } }
        session = null
        runCatching { p.close() }
    }

    internal companion object {
        const val SHORTCUT_ID = "open-voice"

        /** セッションのバスかポータルにつながらない。 */
        const val NO_PORTAL = "no-portal"

        /** ポータルはあるが GlobalShortcuts が無い(古い・デスクトップ環境が対応していない)。 */
        const val NO_GLOBAL_SHORTCUTS = "no-global-shortcuts"

        /** 確認の画面で人が取り消した。 */
        const val CANCELLED = "cancelled"

        fun sessionHandle(r: PortalResponse): String? {
            if (r.code != PortalResponse.SUCCESS) return null
            return when (val h = r.results["session_handle"]) {
                is String -> h
                is DBusObjectPath -> h.path
                else -> null
            }
        }

        fun failure(r: PortalResponse): HotkeyRegistration.Failed =
            if (r.code == PortalResponse.CANCELLED) {
                HotkeyRegistration.Failed(HotkeyFailure.ERROR, CANCELLED)
            } else {
                HotkeyRegistration.Failed(HotkeyFailure.ERROR, "portal response ${r.code}")
            }
    }
}

/** セッションのバスへの最小の接続(AUTH EXTERNAL・Hello・呼び出しと信号)。 */
internal class DBusConnection private constructor(private val channel: SocketChannel) : AutoCloseable {
    private val input: InputStream = Channels.newInputStream(channel)
    private val output: OutputStream = Channels.newOutputStream(channel)
    private val serials = AtomicLong(0)
    private val pending = ConcurrentHashMap<Long, CompletableFuture<DBusMessage>>()
    private val listeners = CopyOnWriteArrayList<(DBusMessage) -> Unit>()

    /** Hello で受け取った自分の名前(`:1.42`)。 */
    var uniqueName: String = ""
        private set

    private fun authenticate(uid: Long) {
        output.write(0)
        output.write("AUTH EXTERNAL ${DBusAddress.externalAuth(uid)}\r\n".toByteArray(Charsets.US_ASCII))
        output.flush()
        val line = readLine()
        if (!line.startsWith("OK")) throw IllegalStateException("D-Bus auth rejected")
        output.write("BEGIN\r\n".toByteArray(Charsets.US_ASCII))
        output.flush()
    }

    private fun readLine(): String {
        val sb = StringBuilder()
        while (true) {
            val b = input.read()
            if (b < 0) throw IllegalStateException("D-Bus closed during auth")
            if (b == '\n'.code) return sb.toString().trimEnd('\r')
            sb.append(b.toChar())
        }
    }

    private fun startReader() {
        val t = Thread({
            try {
                while (true) {
                    val head = input.readNBytes(16)
                    if (head.size < 16) break
                    val rest = input.readNBytes(DBusMessage.totalLength(head) - 16)
                    val m = DBusMessage.decode(head + rest)
                    m.replySerial?.let { pending.remove(it)?.complete(m) }
                    if (m.type == DBusMessage.SIGNAL) listeners.forEach { runCatching { it(m) } }
                }
            } catch (_: Exception) {
                // 閉じた・壊れた: 待っている呼び出しを全部終わらせる
            } finally {
                pending.values.forEach { it.completeExceptionally(IllegalStateException("D-Bus closed")) }
                pending.clear()
            }
        }, "dbus-reader")
        t.isDaemon = true
        t.start()
    }

    fun addSignalListener(l: (DBusMessage) -> Unit) {
        listeners += l
    }

    /** 呼んで返事を待つ。時間切れ・切れたら null。 */
    fun call(
        destination: String,
        path: String,
        iface: String,
        member: String,
        signature: String = "",
        body: List<Any?> = emptyList(),
        timeoutMillis: Long = 10_000,
    ): DBusMessage? {
        val serial = serials.incrementAndGet()
        val f = CompletableFuture<DBusMessage>()
        pending[serial] = f
        val m = DBusMessage(DBusMessage.METHOD_CALL, serial, path, iface, member, destination = destination, signature = signature, body = body)
        return try {
            synchronized(output) {
                output.write(m.encode())
                output.flush()
            }
            f.get(timeoutMillis, TimeUnit.MILLISECONDS)
        } catch (_: Exception) {
            null
        } finally {
            pending.remove(serial)
        }
    }

    fun addMatch(rule: String) {
        call(BUS, BUS_PATH, BUS, "AddMatch", "s", listOf(rule))
    }

    override fun close() {
        runCatching { channel.close() }
    }

    companion object {
        private const val BUS = "org.freedesktop.DBus"
        private const val BUS_PATH = "/org/freedesktop/DBus"

        private interface LibC : Library {
            fun getuid(): Int
        }

        /** `$DBUS_SESSION_BUS_ADDRESS` の unix:path に入る。開けなければ null。 */
        fun session(env: (String) -> String? = System::getenv): DBusConnection? {
            val path = DBusAddress.unixPath(env("DBUS_SESSION_BUS_ADDRESS")) ?: return null
            val uid = runCatching { Native.load("c", LibC::class.java).getuid().toLong() and 0xFFFFFFFFL }.getOrNull() ?: return null
            val ch = runCatching { SocketChannel.open(StandardProtocolFamily.UNIX).apply { connect(UnixDomainSocketAddress.of(path)) } }.getOrNull() ?: return null
            val c = DBusConnection(ch)
            return try {
                c.authenticate(uid)
                c.startReader()
                val hello = c.call(BUS, BUS_PATH, BUS, "Hello") ?: throw IllegalStateException("no Hello reply")
                c.uniqueName = hello.body.firstOrNull() as? String ?: throw IllegalStateException("bad Hello reply")
                c
            } catch (e: Exception) {
                c.close()
                null
            }
        }
    }
}

/** 本物のポータル(D-Bus)。Linux の Wayland でだけ使う。 */
internal class DBusShortcutsPortal(
    private val bus: DBusConnection,
    private val bindTimeoutMillis: Long = 120_000,
) : ShortcutsPortal {
    private val tokens = AtomicLong(0)
    private val responses = ConcurrentHashMap<String, CompletableFuture<PortalResponse>>()

    @Volatile private var activated: ((String, String) -> Unit)? = null

    init {
        bus.addSignalListener { m ->
            val path = m.path ?: return@addSignalListener
            when {
                m.iface == REQUEST && m.member == "Response" -> responses.computeIfAbsent(path) { CompletableFuture() }.complete(response(m.body))
                m.iface == SHORTCUTS && m.member == "Activated" -> {
                    val session = (m.body.getOrNull(0) as? DBusObjectPath)?.path ?: return@addSignalListener
                    val id = m.body.getOrNull(1) as? String ?: return@addSignalListener
                    activated?.invoke(session, id)
                }
            }
        }
        bus.addMatch("type='signal',interface='$REQUEST',member='Response'")
        bus.addMatch("type='signal',interface='$SHORTCUTS',member='Activated'")
    }

    override fun version(): Int? {
        val r = bus.call(PORTAL, PORTAL_PATH, "org.freedesktop.DBus.Properties", "Get", "ss", listOf(SHORTCUTS, "version")) ?: return null
        if (r.type != DBusMessage.METHOD_RETURN) return null
        return ((r.body.firstOrNull() as? DBusVariant)?.value as? Number)?.toInt()
    }

    override fun createSession(): PortalResponse = request("CreateSession", "a{sv}", 10_000) { token ->
        listOf(mapOf("handle_token" to DBusVariant("s", token), "session_handle_token" to DBusVariant("s", "${token}_session")))
    }

    override fun bind(session: String, id: String, description: String, trigger: String?): PortalResponse =
        request("BindShortcuts", "oa(sa{sv})sa{sv}", bindTimeoutMillis) { token ->
            val props = buildMap<String, DBusVariant> {
                put("description", DBusVariant("s", description))
                trigger?.let { put("preferred_trigger", DBusVariant("s", it)) }
            }
            listOf(DBusObjectPath(session), listOf(listOf(id, props)), "", mapOf("handle_token" to DBusVariant("s", token)))
        }

    override fun onActivated(listener: (session: String, id: String) -> Unit) {
        activated = listener
    }

    override fun closeSession(session: String) {
        bus.call(PORTAL, session, "org.freedesktop.portal.Session", "Close", timeoutMillis = 2_000)
    }

    override fun close() = bus.close()

    /** Request を作る呼び出し: 先に Response を待つ場所を作ってから呼ぶ(信号が先に来ても落とさない)。 */
    private fun request(method: String, signature: String, timeoutMillis: Long, body: (token: String) -> List<Any?>): PortalResponse {
        val token = "tailorit_${tokens.incrementAndGet()}"
        val expected = requestPath(bus.uniqueName, token)
        val waiting = responses.computeIfAbsent(expected) { CompletableFuture() }
        var actual = expected
        return try {
            val reply = bus.call(PORTAL, PORTAL_PATH, SHORTCUTS, method, signature, body(token))
            if (reply == null || reply.type != DBusMessage.METHOD_RETURN) return PortalResponse(PortalResponse.OTHER)
            actual = (reply.body.firstOrNull() as? DBusObjectPath)?.path ?: expected
            val f = if (actual == expected) waiting else responses.computeIfAbsent(actual) { CompletableFuture() }
            f.get(timeoutMillis, TimeUnit.MILLISECONDS)
        } catch (_: Exception) {
            PortalResponse(PortalResponse.OTHER)
        } finally {
            responses.remove(expected)
            responses.remove(actual)
        }
    }

    companion object {
        private const val PORTAL = "org.freedesktop.portal.Desktop"
        private const val PORTAL_PATH = "/org/freedesktop/portal/desktop"
        private const val SHORTCUTS = "org.freedesktop.portal.GlobalShortcuts"
        private const val REQUEST = "org.freedesktop.portal.Request"

        /** ポータルが作る Request の path(`:1.42` → `1_42`)。 */
        fun requestPath(uniqueName: String, token: String): String =
            "/org/freedesktop/portal/desktop/request/${uniqueName.removePrefix(":").replace('.', '_')}/$token"

        /** Response(u code, a{sv} results)。 */
        fun response(body: List<Any?>): PortalResponse {
            val code = body.getOrNull(0) as? Long ?: PortalResponse.OTHER
            val results = (body.getOrNull(1) as? Map<*, *>).orEmpty().entries.associate { (k, v) -> k.toString() to (v as? DBusVariant)?.value }
            return PortalResponse(code, results)
        }

        fun connect(osName: String = System.getProperty("os.name").orEmpty(), env: (String) -> String? = System::getenv): ShortcutsPortal? {
            if (!osName.startsWith("Linux", ignoreCase = true)) return null
            val bus = DBusConnection.session(env) ?: return null
            return runCatching { DBusShortcutsPortal(bus) }.getOrElse {
                bus.close()
                null
            }
        }
    }
}
