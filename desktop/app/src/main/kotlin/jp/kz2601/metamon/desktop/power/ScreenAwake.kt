package jp.kz2601.metamon.desktop.power

/** OS に「画面を消さない」を伝える口。テストでは偽物に差し替える。 */
interface ScreenAwakeApi {
    fun setAwake(on: Boolean)
}

/** 何もしない(Windows 以外)。 */
object NoopScreenAwakeApi : ScreenAwakeApi {
    override fun setAwake(on: Boolean) = Unit
}

/**
 * 画面を消さない、を使う所ごとに持つ(iOS の `ScreenAwake` と同じ数え方)。
 * 持っている所が 1 つでもあり、かつ見えているウィンドウが 1 枚でもある間だけ OS に効かせる。
 * 最小化・トレイに隠した・閉じたウィンドウは見えていない扱いで、全部そうなったら戻す。
 */
class ScreenAwake(private val api: ScreenAwakeApi) {
    private var holders = 0
    private val shownWindows = HashSet<String>()
    private var applied = false

    /** 持つ。やめる時は返り値の [Hold.release](何度呼んでもよい)。 */
    @Synchronized
    fun hold(): Hold {
        holders++
        sync()
        return Hold()
    }

    /** ウィンドウが見えているか(最小化・隠した・閉じたら false)。 */
    @Synchronized
    fun setWindowShown(windowId: String, shown: Boolean) {
        if (shown) shownWindows += windowId else shownWindows -= windowId
        sync()
    }

    /** アプリ終了などで、持っている物ごと戻す。 */
    @Synchronized
    fun releaseAll() {
        holders = 0
        shownWindows.clear()
        sync()
    }

    @Synchronized
    private fun drop() {
        if (holders > 0) holders--
        sync()
    }

    private fun sync() {
        val want = holders > 0 && shownWindows.isNotEmpty()
        if (want == applied) return
        applied = want
        runCatching { api.setAwake(want) }
    }

    inner class Hold {
        private var released = false

        fun release() {
            val first = synchronized(this@ScreenAwake) { !released.also { released = true } }
            if (first) drop()
        }
    }

    companion object {
        /** 実機用。Windows だけ効かせる。 */
        val shared: ScreenAwake by lazy {
            ScreenAwake(if (WindowsScreenAwake.isSupported) WindowsScreenAwake else NoopScreenAwakeApi)
        }

        /** タイマーのように動いている間は見ていたい物。iOS の使い所(動いている timerDisplay)に合わせる。 */
        fun timerWantsAwake(isRunning: Boolean): Boolean = isRunning
    }
}
