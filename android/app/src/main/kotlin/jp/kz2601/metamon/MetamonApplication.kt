package jp.kz2601.metamon

import android.app.Application

class MetamonApplication : Application() {
    lateinit var container: AppContainer
        private set

    override fun onCreate() {
        super.onCreate()
        container = AppContainer.make(this)
    }
}
