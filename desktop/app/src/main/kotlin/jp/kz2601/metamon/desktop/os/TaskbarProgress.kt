package jp.kz2601.metamon.desktop.os

import com.sun.jna.Function
import com.sun.jna.Native
import com.sun.jna.Pointer
import com.sun.jna.Structure
import com.sun.jna.ptr.PointerByReference
import com.sun.jna.win32.StdCallLibrary

/** 動いている timerDisplay 1 つの進み具合の元(秒)。[totalSeconds] が null ならストップウォッチ(進み具合は出せない)。 */
data class TimerProgress(val remainingSeconds: Int, val totalSeconds: Int?, val running: Boolean)

/** タスクバーのボタンに出す物。 */
sealed interface TaskbarState {
    /** 何も出さない(止まった・無い) */
    data object None : TaskbarState

    /** 終わるまでの割合(countdown)。[completed] / [total] 秒 */
    data class Normal(val completed: Long, val total: Long) : TaskbarState

    /** 終わりが無い(ストップウォッチ)。動いていることだけ出す */
    data object Indeterminate : TaskbarState
}

/** OS ごとの出し先(Windows は ITaskbarList3)。テストでは作り物に替える。 */
fun interface TaskbarBackend {
    fun show(state: TaskbarState)
}

/**
 * Windows のタスクバーのボタンに、動いているタイマーの進み具合を出す(#3010・ITaskbarList3::SetProgressValue)。
 * iOS のライブアクティビティ・Android の進行中の通知の代わり。動いている物のうち最初の 1 つだけ出し、止まったら消す。
 * 同じ状態は送り直さない。出し先が無い OS(Windows 以外)では何もしない。
 */
class TaskbarProgress(private val backend: TaskbarBackend?) {
    private var last: TaskbarState = TaskbarState.None

    /** タイマーの今の一覧から出し直す。 */
    @Synchronized
    fun update(timers: List<TimerProgress>) {
        val next = stateFor(timers)
        if (next == last) return
        last = next
        runCatching { backend?.show(next) }
    }

    /** 全部消す(ウィンドウを閉じる・終了する時)。 */
    @Synchronized
    fun clear() {
        if (last == TaskbarState.None) return
        last = TaskbarState.None
        runCatching { backend?.show(TaskbarState.None) }
    }

    companion object {
        /** 動いている最初の 1 つ。止まっている・終わった物は出さない。 */
        fun stateFor(timers: List<TimerProgress>): TaskbarState {
            val t = timers.firstOrNull { it.running } ?: return TaskbarState.None
            val total = t.totalSeconds ?: return TaskbarState.Indeterminate
            if (total <= 0) return TaskbarState.None
            val remaining = t.remainingSeconds.coerceIn(0, total)
            if (remaining == 0) return TaskbarState.None
            return TaskbarState.Normal(completed = (total - remaining).toLong(), total = total.toLong())
        }

        /** この OS の出し先。Windows なら [hwnd](ウィンドウのハンドル)に出す。それ以外・ハンドルが無い時は何もしない物。 */
        fun forCurrentOs(hwnd: () -> Long?): TaskbarProgress {
            val isWindows = System.getProperty("os.name").orEmpty().startsWith("Windows", ignoreCase = true)
            return TaskbarProgress(if (isWindows) WindowsTaskbarBackend(hwnd) else null)
        }
    }
}

/**
 * ITaskbarList3 を JNA で呼ぶ(Windows 専用)。COM の呼び出しは vtable の番号で行う
 * (IUnknown 0-2・ITaskbarList 3-7・ITaskbarList2 8・ITaskbarList3 の SetProgressValue = 9・SetProgressState = 10)。
 * 載らない・呼べない時は黙って何もしない(進み具合が出ないだけ)。
 */
internal class WindowsTaskbarBackend(private val hwnd: () -> Long?) : TaskbarBackend {
    private val list: Pointer? by lazy { runCatching { create() }.getOrNull() }

    override fun show(state: TaskbarState) {
        val handle = hwnd() ?: return
        val l = list ?: return
        val window = Pointer.createConstant(handle)
        runCatching {
            when (state) {
                TaskbarState.None -> setState(l, window, TBPF_NOPROGRESS)
                TaskbarState.Indeterminate -> setState(l, window, TBPF_INDETERMINATE)
                is TaskbarState.Normal -> {
                    setState(l, window, TBPF_NORMAL)
                    call(l, SET_PROGRESS_VALUE, l, window, state.completed, state.total)
                }
            }
        }
    }

    private fun setState(l: Pointer, window: Pointer, flag: Int) {
        call(l, SET_PROGRESS_STATE, l, window, flag)
    }

    private fun call(l: Pointer, index: Int, vararg args: Any): Int {
        val vtable = l.getPointer(0)
        val fn = Function.getFunction(vtable.getPointer(index.toLong() * Native.POINTER_SIZE), Function.ALT_CONVENTION)
        return fn.invokeInt(args)
    }

    private fun create(): Pointer? {
        val ole = Native.load("ole32", Ole32::class.java)
        ole.CoInitializeEx(null, COINIT_APARTMENTTHREADED)
        val out = PointerByReference()
        val hr = ole.CoCreateInstance(CLSID_TASKBAR_LIST, null, CLSCTX_INPROC_SERVER, IID_TASKBAR_LIST3, out)
        if (hr != 0) return null
        val l = out.value ?: return null
        if (call(l, HR_INIT, l) != 0) return null
        return l
    }

    @Suppress("FunctionName")
    internal interface Ole32 : StdCallLibrary {
        fun CoInitializeEx(reserved: Pointer?, coInit: Int): Int
        fun CoCreateInstance(clsid: Guid, outer: Pointer?, context: Int, iid: Guid, out: PointerByReference): Int
    }

    @Structure.FieldOrder("data1", "data2", "data3", "data4")
    class Guid(d1: Int = 0, d2: Short = 0, d3: Short = 0, d4: ByteArray = ByteArray(8)) : Structure() {
        @JvmField var data1: Int = d1
        @JvmField var data2: Short = d2
        @JvmField var data3: Short = d3
        @JvmField var data4: ByteArray = d4
    }

    private companion object {
        const val COINIT_APARTMENTTHREADED = 2
        const val CLSCTX_INPROC_SERVER = 1
        const val HR_INIT = 3
        const val SET_PROGRESS_VALUE = 9
        const val SET_PROGRESS_STATE = 10
        const val TBPF_NOPROGRESS = 0
        const val TBPF_INDETERMINATE = 1
        const val TBPF_NORMAL = 2

        /** {56FDF344-FD6D-11d0-958A-006097C9A090} */
        val CLSID_TASKBAR_LIST = Guid(0x56FDF344, 0xFD6D.toShort(), 0x11d0, byteArrayOf(0x95.toByte(), 0x8A.toByte(), 0x00, 0x60, 0x97.toByte(), 0xC9.toByte(), 0xA0.toByte(), 0x90.toByte()))

        /** {EA1AFB91-9E28-4B86-90E9-9E9F8A5EEA84} */
        val IID_TASKBAR_LIST3 = Guid(0xEA1AFB91.toInt(), 0x9E28.toShort(), 0x4B86, byteArrayOf(0x90.toByte(), 0xE9.toByte(), 0x9E.toByte(), 0x9F.toByte(), 0x8A.toByte(), 0x5E.toByte(), 0xEA.toByte(), 0x84.toByte()))
    }
}
