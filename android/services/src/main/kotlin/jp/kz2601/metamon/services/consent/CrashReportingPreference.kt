package jp.kz2601.metamon.services.consent

import jp.kz2601.metamon.services.CrashReporter
import jp.kz2601.metamon.services.NoopCrashReporter
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

/**
 * 設定の「クラッシュ報告を送る」(iOS の `CrashReportingPreference`・#888)。既定はオン(プライバシーポリシー 1.8)。
 * 端末に保存し、変えたらすぐ送り先([CrashReporter])に反映する。オフの間は uid も渡さない。
 */
class CrashReportingPreference(private val store: KeyValueStore) {
    private val _enabled = MutableStateFlow(store.getBoolean(ENABLED_KEY) ?: true)
    val enabled: StateFlow<Boolean> = _enabled.asStateFlow()
    val isEnabled: Boolean get() = _enabled.value

    private var reporter: CrashReporter = NoopCrashReporter
    private var uid: String? = null

    /** 起動時に 1 回: 送り先をつなぎ、保存済みの設定を反映する。 */
    fun activate(reporter: CrashReporter) {
        this.reporter = reporter
        apply()
    }

    fun setEnabled(enabled: Boolean) {
        store.putBoolean(ENABLED_KEY, enabled)
        _enabled.value = enabled
        apply()
    }

    /** サインイン中の uid(サインアウトで null)。 */
    fun updateUser(uid: String?) {
        this.uid = uid
        reporter.setUserId(if (isEnabled) uid else null)
    }

    private fun apply() {
        reporter.setCollectionEnabled(isEnabled)
        reporter.setUserId(if (isEnabled) uid else null)
    }

    companion object {
        const val ENABLED_KEY = "metamon.crashReporting.enabled"
    }
}
