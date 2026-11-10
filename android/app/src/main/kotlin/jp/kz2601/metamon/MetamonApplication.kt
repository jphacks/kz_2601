package jp.kz2601.metamon

import android.app.Application
import androidx.lifecycle.DefaultLifecycleObserver
import androidx.lifecycle.LifecycleOwner
import androidx.lifecycle.ProcessLifecycleOwner
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.launch

class MetamonApplication : Application() {
    private var built: AppContainer? = null

    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)

    override fun onCreate() {
        super.onCreate()
        // 前面の間だけホットリロードを購読する(iOS の scenePhase)。背面では止め、戻ったら張り直して読み直す。
        // 組み立ては最初の画面の起動時なので、まだ無ければ何もしない(組み立てた時は前面)
        ProcessLifecycleOwner.get().lifecycle.addObserver(object : DefaultLifecycleObserver {
            override fun onStart(owner: LifecycleOwner) {
                built?.runtime?.setForeground(true)
                // 前の日までの aiButton の集計(#888)。送るのは前の日の分だけなので 1 日 1 回
                built?.let { scope.launch { it.flushAiButtonMetrics() } }
            }

            override fun onStop(owner: LifecycleOwner) {
                built?.runtime?.setForeground(false)
            }
        })
    }
    /** 組み立て済みの時だけ(FCM の受け口など、画面より先に起きる所から使う)。 */
    val builtContainer: AppContainer? get() = built

    /**
     * 最初の画面の起動時に 1 回だけ組み立てる(DEBUG のつまみは起動の intent で来るため・#844)。
     * 2 回目以降の [knobs] は無視する(つまみを変える時はアプリを止めてから起動し直す)。
     * 組み立てた時に記録の同期(#882)を start する。
     */
    fun container(knobs: DebugKnobs): AppContainer =
        built ?: AppContainer.make(this, knobs).also {
            built = it
            it.recordSync.start()
            // クラッシュ報告のオン / オフ(設定・#888)を送り先に反映する。付けるのは uid だけ(サインアウト・オフで外す)
            it.crashReporting.activate(it.crashReporter)
            scope.launch { it.auth.currentUser.collect { user -> it.crashReporting.updateUser(user?.uid) } }
            scope.launch { it.flushAiButtonMetrics() }
        }
}
