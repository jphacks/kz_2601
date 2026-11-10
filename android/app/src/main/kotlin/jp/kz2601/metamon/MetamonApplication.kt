package jp.kz2601.metamon

import android.app.Application
import androidx.lifecycle.DefaultLifecycleObserver
import androidx.lifecycle.LifecycleOwner
import androidx.lifecycle.ProcessLifecycleOwner

class MetamonApplication : Application() {
    lateinit var container: AppContainer
        private set

    override fun onCreate() {
        super.onCreate()
        container = AppContainer.make(this)
        container.recordSync.start()
        // 前面の間だけホットリロードを購読する(iOS の scenePhase)。背面では止め、戻ったら張り直して読み直す
        ProcessLifecycleOwner.get().lifecycle.addObserver(object : DefaultLifecycleObserver {
            override fun onStart(owner: LifecycleOwner) = container.runtime.setForeground(true)
            override fun onStop(owner: LifecycleOwner) = container.runtime.setForeground(false)
        })
    }
}
