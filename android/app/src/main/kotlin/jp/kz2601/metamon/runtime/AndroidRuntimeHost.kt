package jp.kz2601.metamon.runtime

import android.content.ActivityNotFoundException
import android.content.Context
import android.content.Intent
import android.net.Uri

/** Android の端末機能。いまは URL を開くだけ。通知・書き出し・カレンダーなどは機能ごとの Issue で足す。 */
class AndroidRuntimeHost(private val context: Context) : RuntimeHost {
    override suspend fun openUrl(url: String): Boolean = try {
        context.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(url)).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
        true
    } catch (_: ActivityNotFoundException) {
        false
    }
}
