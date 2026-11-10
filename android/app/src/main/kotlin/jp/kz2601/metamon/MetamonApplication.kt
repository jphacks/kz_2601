package jp.kz2601.metamon

import android.app.Application
import androidx.lifecycle.DefaultLifecycleObserver
import androidx.lifecycle.LifecycleOwner
import androidx.lifecycle.ProcessLifecycleOwner

class MetamonApplication : Application() {
    private var built: AppContainer? = null

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
     * 組み立てた時に記録の同期(#882)を start する。
     */
    fun container(knobs: DebugKnobs): AppContainer =
        built ?: AppContainer.make(this, knobs).also {
            built = it
            it.recordSync.start()
        }
}
