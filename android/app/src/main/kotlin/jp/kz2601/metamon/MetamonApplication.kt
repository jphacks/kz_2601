package jp.kz2601.metamon

import android.app.Application
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.launch

class MetamonApplication : Application() {
    lateinit var container: AppContainer
        private set

    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)

    override fun onCreate() {
        super.onCreate()
        container = AppContainer.make(this)
        container.recordSync.start()
        // クラッシュ報告に付けるのは uid だけ(サインアウトで外す)
        scope.launch { container.auth.currentUser.collect { container.crashReporter.setUserId(it?.uid) } }
    }
}
