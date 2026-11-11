package jp.kz2601.metamon.update

import android.content.ActivityNotFoundException
import android.content.Context
import android.content.Intent
import android.net.Uri

/**
 * Google Play のアプリのページを開く(iOS の `AppStoreLink`)。Play In-App Updates は使わない(#906)。
 * サーバーの URL(`androidStoreUrl`)があればそれ、無ければ `market://details?id=` → 開けなければ https のページ。
 */
object PlayStore {
    fun marketUri(packageName: String): String = "market://details?id=$packageName"

    fun webUri(packageName: String): String = "https://play.google.com/store/apps/details?id=$packageName"

    /** 試す順の URL(重複なし)。 */
    fun candidates(packageName: String, serverUrl: String?): List<String> =
        listOfNotNull(serverUrl, marketUri(packageName), webUri(packageName)).distinct()

    fun open(context: Context, serverUrl: String?) {
        // 開けなければ次の候補へ(Play の無い端末は https をブラウザで開く)
        candidates(context.packageName, serverUrl).firstOrNull { openLink(context, it) }
    }
}

/** 外のアプリで URL を開く。開ける先が無ければ false。 */
internal fun openLink(context: Context, url: String): Boolean = try {
    context.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(url)).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
    true
} catch (_: ActivityNotFoundException) {
    false
}
