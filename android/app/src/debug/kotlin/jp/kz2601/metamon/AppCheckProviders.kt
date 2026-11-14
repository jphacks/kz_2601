package jp.kz2601.metamon

import com.google.firebase.appcheck.AppCheckProviderFactory
import com.google.firebase.appcheck.debug.DebugAppCheckProviderFactory

/**
 * debug ビルドの App Check: debug provider(#318)。Firebase Console に登録したデバッグ用トークンだけ通る。
 * トークンは初回起動時に logcat に出る(`DebugAppCheckProvider` で検索)。release は src/release の Play Integrity。
 */
object AppCheckProviders {
    fun factory(): AppCheckProviderFactory = DebugAppCheckProviderFactory.getInstance()
}
