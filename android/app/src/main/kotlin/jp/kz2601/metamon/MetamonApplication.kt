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
            }

            override fun onStop(owner: LifecycleOwner) {
                built?.runtime?.setForeground(false)
            }
        })
    }

    /**
     * 最初の画面の起動時に 1 回だけ組み立てる(DEBUG のつまみは起動の intent で来るため・#844)。
     * 2 回目以降の [knobs] は無視する(つまみを変える時はアプリを止めてから起動し直す)。
     * 組み立てた時に持ち主の見張り(#841)を start する。記録の同期(#882)はサインインして持ち主を受け入れた後に
     * 見張りが start する(未サインインでは動かさない)。
     */
    fun container(knobs: DebugKnobs): AppContainer =
        built ?: AppContainer.make(this, knobs).also {
            built = it
            it.accountGuard.start(scope)
            // クラッシュ報告に付けるのは uid だけ(サインアウトで外す)
            scope.launch { it.auth.currentUser.collect { user -> it.crashReporter.setUserId(user?.uid) } }
        }
}
