package jp.kz2601.metamon.desktop.platform

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.setValue
import com.sun.jna.Function
import com.sun.jna.Memory
import com.sun.jna.Native
import com.sun.jna.Pointer
import com.sun.jna.WString
import com.sun.jna.ptr.PointerByReference
import jp.kz2601.metamon.desktop.HostOs
import java.io.File
import java.util.UUID

// Windows の「送る」(#3209)。エクスプローラーで選んだファイルを「送る → Tailor It」で、ドラッグ & ドロップと同じ振り分け
// (写真・動画・CSV・PDF)に渡す。ショートカットの引数は [SendToArgs.FLAG] だけで、選んだファイルはその後ろに付いて届く。
// 起動中なら SingleInstance の handOff で前の方へ渡る(引数は 8 つまでなので、送れるのは 7 つまで)。

object SendToArgs {
    const val FLAG = "--send-to"

    /** 送られたファイル(印の後ろの全部)。印が無ければ null。 */
    fun files(argv: List<String>): List<File>? {
        val i = argv.indexOf(FLAG)
        if (i < 0) return null
        return argv.drop(i + 1).filter { it.isNotBlank() }.map(::File)
    }

    /** 印とファイルを除いた引数(アプリの id・記録の JSON・最初の画面の並びは崩さない)。 */
    fun strip(argv: List<String>): List<String> {
        val i = argv.indexOf(FLAG)
        return if (i < 0) argv else argv.take(i)
    }

    /** 読めるファイルだけ(消えた・フォルダ・読めない物は落とす)。 */
    fun readable(files: List<File>, ok: (File) -> Boolean = { it.isFile && it.canRead() }): List<File> = files.filter(ok).distinctBy { it.absolutePath }
}

/**
 * 送られたファイルの受け口。Main が置き、行き先のウィンドウ(DesktopApp)が 1 回だけ受け取って振り分けに渡す。
 * 行き先のウィンドウが記録を読み終える前に届いても、画面ができた後に受け取れる。
 */
class SendToInbox {
    /** 増えたら DesktopApp が [take] を試す。 */
    var requests by mutableIntStateOf(0)
        private set

    private var pending: List<File> = emptyList()
    private var target: Int? = null

    @Synchronized
    fun offer(files: List<File>, windowId: Int?) {
        if (files.isEmpty()) return
        pending = pending + files
        target = windowId
        requests++
    }

    /** [windowId] が行き先(行き先が無ければ前にあるウィンドウ)なら、届いた分を返して空にする。 */
    @Synchronized
    fun take(windowId: Int?, isActive: () -> Boolean): List<File> {
        if (pending.isEmpty()) return emptyList()
        val t = target
        if (if (t != null) windowId != t else !isActive()) return emptyList()
        return pending.also {
            pending = emptyList()
            target = null
        }
    }
}

/** 「送る」のフォルダにショートカットを置く口。Windows 以外・入れた物でない(開発の起動)時は [Noop]。 */
interface SendToShortcut {
    /** 置いた・もう置いてあれば true。 */
    fun ensure(): Boolean

    object Noop : SendToShortcut {
        override fun ensure(): Boolean = false
    }

    companion object {
        const val LINK_NAME = "Tailor It.lnk"

        /** `%APPDATA%\Microsoft\Windows\SendTo\Tailor It.lnk`。 */
        fun linkFile(appData: String?): File? = appData?.takeIf { it.isNotBlank() }?.let { File(it, "Microsoft\\Windows\\SendTo\\$LINK_NAME".replace('\\', File.separatorChar)) }

        fun create(
            os: HostOs = HostOs.current,
            appPath: String? = System.getProperty("jpackage.app-path"),
            appData: String? = System.getenv("APPDATA"),
        ): SendToShortcut {
            if (os != HostOs.WINDOWS || appPath == null) return Noop
            val link = linkFile(appData) ?: return Noop
            return WindowsSendToShortcut(appPath, link)
        }
    }
}

/** IShellLinkW + IPersistFile を JNA で(jna-platform は足さず、COM の表を番号で呼ぶ)。 */
internal class WindowsSendToShortcut(private val exe: String, private val link: File) : SendToShortcut {
    override fun ensure(): Boolean = runCatching {
        if (link.isFile) return true
        link.parentFile?.takeIf { it.isDirectory } ?: return false
        save()
    }.getOrElse {
        System.err.println("[TailorIt] send to: $it")
        false
    }

    private fun save(): Boolean {
        val init = ole32("CoInitializeEx").invokeInt(arrayOf(null, 0x2))
        try {
            val out = PointerByReference()
            if (ole32("CoCreateInstance").invokeInt(arrayOf(guid(CLSID_SHELL_LINK), null, 0x1, guid(IID_SHELL_LINK_W), out)) < 0) return false
            val sl = out.value ?: return false
            try {
                // SetPath(20)・SetArguments(11)・SetIconLocation(17)
                if (call(sl, 20, WString(exe)) < 0 || call(sl, 11, WString(SendToArgs.FLAG)) < 0 || call(sl, 17, WString(exe), 0) < 0) return false
                val pf = PointerByReference()
                if (call(sl, 0, guid(IID_PERSIST_FILE), pf) < 0) return false
                val p = pf.value ?: return false
                try {
                    // IPersistFile::Save(path, TRUE)
                    return call(p, 6, WString(link.absolutePath), 1) >= 0
                } finally {
                    call(p, 2)
                }
            } finally {
                call(sl, 2)
            }
        } finally {
            if (init >= 0) ole32("CoUninitialize").invokeVoid(arrayOf())
        }
    }

    private fun call(obj: Pointer, index: Int, vararg args: Any?): Int {
        val fn = Function.getFunction(obj.getPointer(0).getPointer(index.toLong() * Native.POINTER_SIZE), Function.ALT_CONVENTION)
        return fn.invokeInt(arrayOf(obj, *args))
    }

    private fun ole32(name: String): Function = Function.getFunction("ole32", name, Function.ALT_CONVENTION)

    companion object {
        const val CLSID_SHELL_LINK = "00021401-0000-0000-c000-000000000046"
        const val IID_SHELL_LINK_W = "000214f9-0000-0000-c000-000000000046"
        const val IID_PERSIST_FILE = "0000010b-0000-0000-c000-000000000046"

        /** GUID の並び(Data1〜3 はリトルエンディアン・Data4 はそのまま)。 */
        fun guidBytes(s: String): ByteArray {
            val u = UUID.fromString(s)
            val hi = u.mostSignificantBits
            val lo = u.leastSignificantBits
            val d1 = (hi ushr 32).toInt()
            val d2 = (hi ushr 16).toInt() and 0xffff
            val d3 = hi.toInt() and 0xffff
            return ByteArray(16).also { out ->
                for (i in 0 until 4) out[i] = (d1 ushr (8 * i)).toByte()
                out[4] = d2.toByte(); out[5] = (d2 ushr 8).toByte()
                out[6] = d3.toByte(); out[7] = (d3 ushr 8).toByte()
                for (i in 0 until 8) out[8 + i] = (lo ushr (8 * (7 - i))).toByte()
            }
        }

        private fun guid(s: String): Memory = Memory(16).apply { write(0, guidBytes(s), 0, 16) }
    }
}
