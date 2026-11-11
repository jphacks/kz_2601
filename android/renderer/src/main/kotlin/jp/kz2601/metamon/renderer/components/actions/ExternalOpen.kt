package jp.kz2601.metamon.renderer.components.actions

import android.content.ActivityNotFoundException
import android.content.Context
import android.content.Intent
import android.net.Uri
import androidx.browser.customtabs.CustomTabsIntent
import jp.kz2601.metamon.dsl.logic.ExternalUrlPolicy
import jp.kz2601.metamon.dsl.logic.MapsUrl

/** 外へ開く時の決まり(Android の Intent に依らない部分。JVM で試す)。 */
object ExternalOpen {
    /**
     * 開く候補を Android で開ける形にする: 許可していないスキームは落とし、Apple の地図・iOS の Google マップの URL は
     * Google マップの URL に写す([MapsUrl])。順番は保つ(先頭から試し、開けなければ次)。
     */
    fun candidates(urls: List<String>): List<String> =
        urls.filter(ExternalUrlPolicy::isAllowed).map(MapsUrl::forAndroid).distinct()

    /** アプリ内のブラウザ(Custom Tabs)で開くか。サービスのリンク・地図はアプリに渡したいので [preferApp] の時は開かない。 */
    fun usesCustomTab(url: String, preferApp: Boolean): Boolean =
        !preferApp && ExternalUrlPolicy.scheme(url) == "https"

    /** `mailto:` は送信先を選ぶ画面(ACTION_SENDTO)、それ以外は ACTION_VIEW。 */
    fun intentAction(url: String): String =
        if (ExternalUrlPolicy.scheme(url) == "mailto") Intent.ACTION_SENDTO else Intent.ACTION_VIEW
}

/**
 * 候補を先頭から開く。開けるアプリが無ければ次を試す(slack:// が無い端末は Slack の Web)。
 * [preferApp]: https でも Custom Tabs にせず、対応アプリ(App Links)があればそちらで開く(openUrlButton の app・共有の URL)。
 * @return 開けたか
 */
fun openExternal(context: Context, urls: List<String>, preferApp: Boolean = false): Boolean {
    for (url in ExternalOpen.candidates(urls)) {
        val uri = Uri.parse(url)
        try {
            if (ExternalOpen.usesCustomTab(url, preferApp)) {
                CustomTabsIntent.Builder().setShowTitle(true).build().launchUrl(context, uri)
            } else {
                context.startActivity(Intent(ExternalOpen.intentAction(url), uri).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
            }
            return true
        } catch (_: ActivityNotFoundException) {
            continue
        }
    }
    return false
}

/** OS の共有シート(ACTION_SEND・text/plain)。 */
fun shareText(context: Context, text: String, title: String? = null) {
    val send = Intent(Intent.ACTION_SEND).setType("text/plain").putExtra(Intent.EXTRA_TEXT, text)
    val chooser = Intent.createChooser(send, title).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
    try {
        context.startActivity(chooser)
    } catch (_: ActivityNotFoundException) {
        // 共有先が 1 つも無い端末では何もしない
    }
}
