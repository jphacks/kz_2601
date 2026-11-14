package jp.kz2601.metamon.desktop.theme

import com.sun.jna.Native
import com.sun.jna.Structure
import com.sun.jna.Pointer
import com.sun.jna.win32.StdCallLibrary

/**
 * OS のコントラストのテーマで使う色(0xRRGGBB)。Windows は GetSysColor から作る。
 * [grayText] は無効の文字、[hotlight] はリンク。
 */
data class SystemColors(
    val window: Int,
    val windowText: Int,
    val highlight: Int,
    val highlightText: Int,
    val grayText: Int,
    val hotlight: Int,
) {
    companion object {
        /** Windows の「ハイコントラスト黒」(OS の色が読めない時の暗い側)。 */
        val ContrastBlack = SystemColors(
            window = 0x000000, windowText = 0xFFFFFF, highlight = 0x1AEBFF, highlightText = 0x000000,
            grayText = 0x3FF23F, hotlight = 0xFFFF00,
        )

        /** Windows の「ハイコントラスト白」(OS の色が読めない時の明るい側)。 */
        val ContrastWhite = SystemColors(
            window = 0xFFFFFF, windowText = 0x000000, highlight = 0x37006E, highlightText = 0xFFFFFF,
            grayText = 0x600000, hotlight = 0x00009F,
        )

        /** GetSysColor の COLORREF(0x00BBGGRR)を 0xRRGGBB にする。 */
        fun fromColorRef(colorRef: Int): Int {
            val r = colorRef and 0xFF
            val g = (colorRef shr 8) and 0xFF
            val b = (colorRef shr 16) and 0xFF
            return (r shl 16) or (g shl 8) or b
        }
    }
}

/** OS のコントラストのテーマ。[on] でなければ [colors] は使わない。[colors] が null なら決まった配色を使う。 */
data class HighContrastState(val on: Boolean, val colors: SystemColors? = null)

/** OS のコントラストのテーマを読む口(Windows は JNA・Linux は GNOME の gsettings・他は読まない)。 */
fun interface HighContrastReader {
    /** 読めない時は null。 */
    fun read(): HighContrastState?
}

/**
 * Windows: user32 の `SystemParametersInfoW(SPI_GETHIGHCONTRAST)` で on / off、on なら `GetSysColor` で色を読む。
 * Windows 以外・JNA が載らない時は null。
 */
object WindowsHighContrast : HighContrastReader {
    private const val SPI_GETHIGHCONTRAST = 0x0042
    private const val HCF_HIGHCONTRASTON = 0x00000001
    private const val COLOR_WINDOW = 5
    private const val COLOR_WINDOWTEXT = 8
    private const val COLOR_HIGHLIGHT = 13
    private const val COLOR_HIGHLIGHTTEXT = 14
    private const val COLOR_GRAYTEXT = 17
    private const val COLOR_HOTLIGHT = 26

    private val isWindows = System.getProperty("os.name").orEmpty().startsWith("Windows", ignoreCase = true)

    private val user32: User32? by lazy {
        if (!isWindows) null else runCatching { Native.load("user32", User32::class.java) }.getOrNull()
    }

    override fun read(): HighContrastState? {
        val u = user32 ?: return null
        return runCatching {
            val hc = HighContrastW()
            hc.cbSize = hc.size()
            if (!u.SystemParametersInfoW(SPI_GETHIGHCONTRAST, hc.size(), hc, 0)) return null
            if (!isOn(hc.dwFlags)) return HighContrastState(on = false)
            HighContrastState(
                on = true,
                colors = SystemColors(
                    window = SystemColors.fromColorRef(u.GetSysColor(COLOR_WINDOW)),
                    windowText = SystemColors.fromColorRef(u.GetSysColor(COLOR_WINDOWTEXT)),
                    highlight = SystemColors.fromColorRef(u.GetSysColor(COLOR_HIGHLIGHT)),
                    highlightText = SystemColors.fromColorRef(u.GetSysColor(COLOR_HIGHLIGHTTEXT)),
                    grayText = SystemColors.fromColorRef(u.GetSysColor(COLOR_GRAYTEXT)),
                    hotlight = SystemColors.fromColorRef(u.GetSysColor(COLOR_HOTLIGHT)),
                ),
            )
        }.getOrNull()
    }

    /** HIGHCONTRAST の dwFlags に HCF_HIGHCONTRASTON が立っているか。 */
    internal fun isOn(flags: Int): Boolean = flags and HCF_HIGHCONTRASTON != 0

    @Suppress("FunctionName")
    internal interface User32 : StdCallLibrary {
        fun SystemParametersInfoW(uiAction: Int, uiParam: Int, pvParam: HighContrastW, fWinIni: Int): Boolean
        fun GetSysColor(nIndex: Int): Int
    }

    /** winuser.h の HIGHCONTRASTW。JNA が反射で触るので public。 */
    @Structure.FieldOrder("cbSize", "dwFlags", "lpszDefaultScheme")
    class HighContrastW : Structure() {
        @JvmField var cbSize: Int = 0
        @JvmField var dwFlags: Int = 0
        @JvmField var lpszDefaultScheme: Pointer? = null
    }
}
