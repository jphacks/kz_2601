package jp.kz2601.metamon

import android.app.Application
import android.content.ComponentCallbacks2
import android.util.Log
import jp.kz2601.metamon.lifecycle.AppForeground
import jp.kz2601.metamon.lifecycle.foregroundSteps
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
        // 前面・背面の処理はすべて AppForeground に寄せる(iOS の scenePhase・#891)。
        // 組み立ては最初の画面の起動時なので、まだ無ければ何もしない(組み立てた時は前面)
        ProcessLifecycleOwner.get().lifecycle.addObserver(object : DefaultLifecycleObserver {
            override fun onStart(owner: LifecycleOwner) {
                foreground?.setForeground(true)
                built?.refreshWidget() // 日付が変わっていれば「直近 7 日」を進める(#912)
            }

            override fun onStop(owner: LifecycleOwner) {
                foreground?.setForeground(false)
            }
        })
    }

    private var foreground: AppForeground? = null

    override fun onTrimMemory(level: Int) {
        super.onTrimMemory(level)
        if (releasesOnTrim(level)) built?.releaseHeavyMemory()
    }

    /** 組み立て済みの時だけ(FCM の受け口など、画面より先に起きる所から使う)。 */
    val builtContainer: AppContainer? get() = built

    /**
     * 最初の画面の起動時に 1 回だけ組み立てる(DEBUG のつまみは起動の intent で来るため・#844)。
     * 2 回目以降の [knobs] は無視する(つまみを変える時はアプリを止めてから起動し直す)。
     * 組み立てた時に持ち主の見張り(#841)を start する。記録の同期(#882)はサインインして持ち主を受け入れた後に
     * 見張りが start する(未サインインでは動かさない)。
     */
    fun container(knobs: DebugKnobs): AppContainer =
        built ?: AppContainer.make(this, knobs).also {
            built = it
            foreground = AppForeground(scope, it.foregroundSteps(), log = { line -> Log.w(AppContainer.TAG, line) }).apply { markForeground() }
            it.accountGuard.start(scope)
            it.startSessionExpiry(scope) // トークンを取り直しても 401 ならサインイン画面へ(#1102)
            it.startWidget(scope) // DSL の適用・記録の変化でホーム画面のウィジェットを書き直す(#912)
            // クラッシュ報告のオン / オフ(設定・#888)を送り先に反映する。付けるのは uid だけ(サインアウト・オフで外す)
            it.crashReporting.activate(it.crashReporter)
            scope.launch { it.auth.currentUser.collect { user -> it.crashReporting.updateUser(user?.uid) } }
            scope.launch { it.flushAiButtonMetrics() }
        }

    companion object {
        /**
         * メモリが足りない時だけ重いものを外す。背面に回っただけ(UI_HIDDEN)では外さない(すぐ戻ると読み込み直しになる)。
         * 外さなくても使わなければ 2 分で外れる(WhisperTranscriber.IDLE_UNLOAD_MILLIS)。
         */
        @Suppress("DEPRECATION") // RUNNING_* は API 34 から届かないが、それより前の端末では届く
        fun releasesOnTrim(level: Int): Boolean =
            level != ComponentCallbacks2.TRIM_MEMORY_UI_HIDDEN && level >= ComponentCallbacks2.TRIM_MEMORY_RUNNING_LOW
    }
}
