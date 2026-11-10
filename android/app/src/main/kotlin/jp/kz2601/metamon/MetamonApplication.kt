package jp.kz2601.metamon

import android.app.Application

class MetamonApplication : Application() {
    private var built: AppContainer? = null

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
