package jp.kz2601.metamon.desktop.platform

import com.sun.jna.Function
import com.sun.jna.Memory
import com.sun.jna.Native
import com.sun.jna.Pointer
import com.sun.jna.WString
import com.sun.jna.ptr.IntByReference
import com.sun.jna.ptr.PointerByReference
import jp.kz2601.metamon.desktop.HostOs
import java.util.UUID
import java.util.concurrent.Executors

// Windows のジャンプリスト(#3208)。タスクバーのボタンの右クリックに、最近開いたアプリ・すぐ記録(・話しかける)を出す。
// 既定は出さない(`TAILORIT_JUMPLIST=1` の時だけ)。Windows 以外では何もしない。
// 並び・引数を作る所は純粋([JumpListPlan]・[JumpArgs]・[RecentApps])、OS に出す所は [JumpListPublisher] の後ろ。
// 押された項目は exe を引数つきで起動する。2 つ目の起動になるので SingleInstance.handOff で起動中の方へ渡る。

/** ジャンプリストから来た起動。アプリの項目はアプリの id だけを渡す(前からの引数の形のまま)。 */
enum class JumpCommand(val wire: String) {
    QUICK_RECORD("quick-record"),
    VOICE("voice"),
}

object JumpArgs {
    const val PREFIX = "--jump="

    fun encode(c: JumpCommand): List<String> = listOf(PREFIX + c.wire)

    /** 引数の中のジャンプリストの指示(無ければ null)。 */
    fun parse(args: List<String>): JumpCommand? =
        args.firstNotNullOfOrNull { a -> a.takeIf { it.startsWith(PREFIX) }?.removePrefix(PREFIX)?.let { w -> JumpCommand.entries.firstOrNull { it.wire == w } } }

    /** 指示を除いた引数(アプリの id・記録の JSON・最初の画面の並びを崩さない)。 */
    fun strip(args: List<String>): List<String> = args.filterNot { it.startsWith(PREFIX) }
}

/** 最近開いたアプリ(新しい順)。設定に改行区切りで残す。 */
object RecentApps {
    const val KEY = "tailorit.recentApps"
    const val MAX = 5

    fun touch(current: List<String>, appId: String, max: Int = MAX): List<String> =
        (listOf(appId) + current.filter { it != appId }).take(max)

    fun decode(s: String?): List<String> = s.orEmpty().lines().map { it.trim() }.filter { it.isNotEmpty() }.distinct()

    fun encode(ids: List<String>): String = ids.joinToString("\n")
}

/** ジャンプリストの 1 行。[args] は exe に渡す引数。 */
data class JumpItem(val title: String, val args: List<String>, val description: String = title)

/** OS に出す形: 「最近」の分類と、タスク(すぐ記録など)。 */
data class JumpListContent(val categoryTitle: String, val recent: List<JumpItem>, val tasks: List<JumpItem>) {
    val isEmpty: Boolean get() = recent.isEmpty() && tasks.isEmpty()
}

object JumpListPlan {
    class Labels(val recent: String, val quickRecord: String, val voice: String)

    /**
     * [recent] は新しい順のアプリの id、[names] は同梱のアプリの (id → 名前)。知らない id(消えたアプリ)は出さない。
     * すぐ記録は前にあるアプリに書ける所がある時だけ、話しかけるは音声を使う設定の時だけ。
     */
    fun build(recent: List<String>, names: Map<String, String>, canQuickRecord: Boolean, voice: Boolean, labels: Labels): JumpListContent {
        val apps = recent.filter { it in names }.distinct().take(RecentApps.MAX).map { id -> JumpItem(names.getValue(id), listOf(id)) }
        val tasks = buildList {
            if (canQuickRecord) add(JumpItem(labels.quickRecord, JumpArgs.encode(JumpCommand.QUICK_RECORD)))
            if (voice) add(JumpItem(labels.voice, JumpArgs.encode(JumpCommand.VOICE)))
        }
        return JumpListContent(labels.recent, apps, tasks)
    }

    /** Windows のコマンドラインの 1 つの引数(空白・" を含む時は囲む)。 */
    fun quote(arg: String): String {
        if (arg.isNotEmpty() && arg.none { it == ' ' || it == '\t' || it == '"' }) return arg
        val sb = StringBuilder("\"")
        var backslashes = 0
        for (ch in arg) {
            when (ch) {
                '\\' -> backslashes++
                '"' -> {
                    sb.append("\\".repeat(backslashes * 2 + 1)).append('"')
                    backslashes = 0
                }
                else -> {
                    sb.append("\\".repeat(backslashes)).append(ch)
                    backslashes = 0
                }
            }
        }
        return sb.append("\\".repeat(backslashes * 2)).append('"').toString()
    }

    fun commandLine(args: List<String>): String = args.joinToString(" ", transform = ::quote)
}

/** ジャンプリストを OS に出す口。Windows 以外は [Noop]。 */
interface JumpListPublisher {
    /** 出せたら true。 */
    fun publish(content: JumpListContent): Boolean

    object Noop : JumpListPublisher {
        override fun publish(content: JumpListContent): Boolean = false
    }

    companion object {
        /** `TAILORIT_JUMPLIST=1`(か `-Dtailorit.jumplist=true`)で、Windows の時だけ。 */
        fun enabled(
            os: HostOs = HostOs.current,
            props: (String) -> String? = System::getProperty,
            env: (String) -> String? = System::getenv,
        ): Boolean = os == HostOs.WINDOWS &&
            (props("tailorit.jumplist") ?: env("TAILORIT_JUMPLIST"))?.trim()?.lowercase() in setOf("1", "true", "on", "yes")

        fun create(os: HostOs = HostOs.current, enabled: Boolean = enabled(os)): JumpListPublisher {
            if (!enabled) return Noop
            val exe = System.getProperty("jpackage.app-path") ?: ProcessHandle.current().info().command().orElse(null) ?: return Noop
            return WindowsJumpList(exe)
        }
    }
}

/**
 * ICustomDestinationList を JNA で(jna-platform は足さず、COM の表を番号で呼ぶ)。
 * COM は 1 本のスレッド(STA)で呼ぶ。失敗は false で返し、アプリは止めない。
 */
internal class WindowsJumpList(private val exe: String) : JumpListPublisher {
    private val worker = Executors.newSingleThreadExecutor { r -> Thread(r, "jump-list").apply { isDaemon = true } }

    override fun publish(content: JumpListContent): Boolean = runCatching {
        worker.submit<Boolean> { runCatching { commit(content) }.getOrElse { System.err.println("[TailorIt] jump list: $it"); false } }.get()
    }.getOrDefault(false)

    private fun commit(content: JumpListContent): Boolean {
        val coInit = ole32("CoInitializeEx").invokeInt(arrayOf(null, COINIT_APARTMENTTHREADED))
        try {
            val list = create(CLSID_DESTINATION_LIST, IID_CUSTOM_DESTINATION_LIST) ?: return false
            try {
                val removed = PointerByReference()
                // BeginList(&minSlots, IID_IObjectArray, &removed)
                if (call(list, 4, IntByReference(), guid(IID_OBJECT_ARRAY), removed) < 0) return false
                removed.value?.let { release(it) }
                if (content.recent.isNotEmpty()) {
                    val arr = collection(content.recent) ?: return abort(list)
                    val hr = call(list, 5, WString(content.categoryTitle), arr)
                    release(arr)
                    if (hr < 0) return abort(list)
                }
                if (content.tasks.isNotEmpty()) {
                    val arr = collection(content.tasks) ?: return abort(list)
                    val hr = call(list, 7, arr)
                    release(arr)
                    if (hr < 0) return abort(list)
                }
                return call(list, 8) >= 0
            } finally {
                release(list)
            }
        } finally {
            if (coInit >= 0) ole32("CoUninitialize").invokeVoid(arrayOf())
        }
    }

    private fun abort(list: Pointer): Boolean {
        call(list, 11)
        return false
    }

    /** IObjectCollection に IShellLinkW を詰め、IObjectArray にして返す。 */
    private fun collection(items: List<JumpItem>): Pointer? {
        val coll = create(CLSID_ENUMERABLE_OBJECT_COLLECTION, IID_OBJECT_COLLECTION) ?: return null
        try {
            for (item in items) {
                val link = shellLink(item) ?: return null
                val hr = call(coll, 5, link)
                release(link)
                if (hr < 0) return null
            }
            val arr = PointerByReference()
            if (call(coll, 0, guid(IID_OBJECT_ARRAY), arr) < 0) return null
            return arr.value
        } finally {
            release(coll)
        }
    }

    private fun shellLink(item: JumpItem): Pointer? {
        val link = create(CLSID_SHELL_LINK, IID_SHELL_LINK_W) ?: return null
        val ok = call(link, 20, WString(exe)) >= 0 && // SetPath
            call(link, 11, WString(JumpListPlan.commandLine(item.args))) >= 0 && // SetArguments
            call(link, 7, WString(item.description)) >= 0 && // SetDescription
            call(link, 17, WString(exe), 0) >= 0 && // SetIconLocation
            setTitle(link, item.title)
        if (!ok) {
            release(link)
            return null
        }
        return link
    }

    /** IPropertyStore の PKEY_Title(ジャンプリストに出る名前)。 */
    private fun setTitle(link: Pointer, title: String): Boolean {
        val store = PointerByReference()
        if (call(link, 0, guid(IID_PROPERTY_STORE), store) < 0) return false
        val ps = store.value ?: return false
        try {
            val key = Memory(20).apply {
                write(0, guidBytes(PKEY_TITLE_FMTID), 0, 16)
                setInt(16, 2)
            }
            val text = Memory((title.length + 1) * 2L).apply { setWideString(0, title) }
            val pv = Memory(24).apply {
                clear()
                setShort(0, VT_LPWSTR)
                setPointer(8, text)
            }
            return call(ps, 6, key, pv) >= 0 && call(ps, 7) >= 0
        } finally {
            release(ps)
        }
    }

    private fun create(clsid: String, iid: String): Pointer? {
        val out = PointerByReference()
        val hr = ole32("CoCreateInstance").invokeInt(arrayOf(guid(clsid), null, CLSCTX_INPROC_SERVER, guid(iid), out))
        return if (hr >= 0) out.value else null
    }

    private fun release(p: Pointer) {
        call(p, 2)
    }

    /** COM の表の [index] 番目を呼ぶ(1 つ目の引数は自分)。 */
    private fun call(obj: Pointer, index: Int, vararg args: Any?): Int {
        val fn = Function.getFunction(obj.getPointer(0).getPointer(index.toLong() * Native.POINTER_SIZE), Function.ALT_CONVENTION)
        return fn.invokeInt(arrayOf(obj, *args))
    }

    private fun ole32(name: String): Function = Function.getFunction("ole32", name, Function.ALT_CONVENTION)

    companion object {
        private const val COINIT_APARTMENTTHREADED = 0x2
        private const val CLSCTX_INPROC_SERVER = 0x1
        private const val VT_LPWSTR: Short = 31

        const val CLSID_DESTINATION_LIST = "77f10cf0-3db5-4966-b520-b7c54fd35ed6"
        const val IID_CUSTOM_DESTINATION_LIST = "6332debf-87b5-4670-90c0-5e57b408a49e"
        const val CLSID_ENUMERABLE_OBJECT_COLLECTION = "2d3468c1-36a7-43b6-ac24-d3f02fd9607a"
        const val IID_OBJECT_COLLECTION = "5632b1a4-e38a-400a-928a-d4cd63230295"
        const val IID_OBJECT_ARRAY = "92ca9dcd-5622-4bba-a805-5e9f541bd8c9"
        const val CLSID_SHELL_LINK = "00021401-0000-0000-c000-000000000046"
        const val IID_SHELL_LINK_W = "000214f9-0000-0000-c000-000000000046"
        const val IID_PROPERTY_STORE = "886d8eeb-8cf2-4446-8d02-cdba1dbdcf99"
        const val PKEY_TITLE_FMTID = "f29f85e0-4ff9-1068-ab91-08002b27b3d9"

        /** GUID の並び(Data1〜3 はリトルエンディアン・Data4 はそのまま)。 */
        fun guidBytes(s: String): ByteArray {
            val u = UUID.fromString(s)
            val hi = u.mostSignificantBits
            val lo = u.leastSignificantBits
            val out = ByteArray(16)
            val d1 = (hi ushr 32).toInt()
            val d2 = (hi ushr 16).toInt() and 0xffff
            val d3 = hi.toInt() and 0xffff
            for (i in 0 until 4) out[i] = (d1 ushr (8 * i)).toByte()
            out[4] = d2.toByte(); out[5] = (d2 ushr 8).toByte()
            out[6] = d3.toByte(); out[7] = (d3 ushr 8).toByte()
            for (i in 0 until 8) out[8 + i] = (lo ushr (8 * (7 - i))).toByte()
            return out
        }

        private fun guid(s: String): Memory = Memory(16).apply { write(0, guidBytes(s), 0, 16) }
    }
}
