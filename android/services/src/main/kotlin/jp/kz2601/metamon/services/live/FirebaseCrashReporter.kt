package jp.kz2601.metamon.services.live

import com.google.firebase.crashlytics.FirebaseCrashlytics
import jp.kz2601.metamon.services.CrashReporter

/** Firebase Crashlytics(iOS の `FirebaseCrashReporter`)。 */
class FirebaseCrashReporter private constructor(private val crashlytics: FirebaseCrashlytics) : CrashReporter {
    override fun setCollectionEnabled(enabled: Boolean) {
        crashlytics.isCrashlyticsCollectionEnabled = enabled // SDK が保存するので次の起動の最初から効く
        if (!enabled) crashlytics.deleteUnsentReports() // オフにする前に溜まった分も送らない
    }

    override fun setUserId(uid: String?) {
        crashlytics.setUserId(uid.orEmpty())
    }

    companion object {
        /** Firebase が未初期化なら null(呼ぶ側が [jp.kz2601.metamon.services.NoopCrashReporter] に落とす)。 */
        fun createIfConfigured(): FirebaseCrashReporter? =
            if (FirebaseBootstrap.isConfigured) FirebaseCrashReporter(FirebaseCrashlytics.getInstance()) else null
    }
}
