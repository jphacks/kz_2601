package jp.kz2601.metamon.push

import android.content.Context
import android.os.Build
import com.google.android.gms.tasks.Task
import com.google.firebase.messaging.FirebaseMessaging
import com.google.firebase.messaging.FirebaseMessagingService
import com.google.firebase.messaging.RemoteMessage
import jp.kz2601.metamon.MetamonApplication
import jp.kz2601.metamon.notifications.LocalNotifications
import jp.kz2601.metamon.services.live.FirebaseBootstrap
import jp.kz2601.metamon.services.push.PushMessage
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.launch
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException

/**
 * FCM の受け口(#887)。google-services.json が無いビルドでは FCM が動かないので、ここは呼ばれない。
 * - 前面・data だけのメッセージ: 「他の人の行動」のチャンネルで出す。押すと appId のアプリを開く
 * - 裏で notification つきのメッセージ: OS が既定のチャンネル(AndroidManifest)で出す。押すと data が extra に入って起動する
 * - トークンの更新: 画面が組み立て済みなら送る。まだなら次に前面へ来た時の refresh が今のトークンを取り直す
 */
class MetamonMessagingService : FirebaseMessagingService() {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)

    override fun onNewToken(token: String) {
        val push = (application as? MetamonApplication)?.builtContainer?.push ?: return
        scope.launch { push.receive(token) }
    }

    override fun onMessageReceived(message: RemoteMessage) {
        val m = PushMessage.from(message.data, message.notification?.title, message.notification?.body)
        if (m.isEmpty) return
        LocalNotifications.get(this).post(
            identifier = "push:${message.messageId ?: message.sentTime}",
            title = m.title,
            body = m.body,
            alarm = false,
            channel = LocalNotifications.CHANNEL_OTHERS,
            appId = m.appId,
        )
    }
}

/** FCM のトークン。Firebase が無い構成では null(何もしない)。 */
object FcmTokens {
    suspend fun current(): String? {
        if (!FirebaseBootstrap.isConfigured) return null
        return FirebaseMessaging.getInstance().token.await()
    }

    fun osName(): String = "Android ${Build.VERSION.RELEASE}"

    fun prefs(context: Context) = context.applicationContext.getSharedPreferences("metamon.push", Context.MODE_PRIVATE)
}

private suspend fun <T> Task<T>.await(): T = suspendCancellableCoroutine { cont ->
    addOnSuccessListener { cont.resume(it) }
    addOnFailureListener { cont.resumeWithException(it) }
    addOnCanceledListener { cont.cancel() }
}
