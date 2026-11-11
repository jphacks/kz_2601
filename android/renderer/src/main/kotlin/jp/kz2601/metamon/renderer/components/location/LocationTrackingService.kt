package jp.kz2601.metamon.renderer.components.location

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Intent
import android.content.pm.ServiceInfo
import android.location.LocationListener
import android.location.LocationManager
import android.os.Handler
import android.os.IBinder
import android.os.Looper
import androidx.core.app.NotificationCompat
import jp.kz2601.metamon.renderer.R

/**
 * 位置の記録の Foreground Service(type `location`・#869)。[LocationTracking] が前面で開始し、停止で止める。
 * 停止ボタンつきの通知を常に出す(iOS の青い位置インジケータの代わり)。プロセスが落ちたら再開しない(START_NOT_STICKY)。
 */
class LocationTrackingService : Service() {
    private var manager: LocationManager? = null
    private val listener = LocationListener { LocationTracking.onLocation(it) }
    private val handler = Handler(Looper.getMainLooper())
    private val ticker = object : Runnable {
        override fun run() {
            LocationTracking.tick()
            handler.postDelayed(this, TICK_MS)
        }
    }

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        if (intent?.action == ACTION_STOP) {
            LocationTracking.stop()
            stopSelf()
            return START_NOT_STICKY
        }
        // startForegroundService で起きたので、まず前面にする(しないと OS がアプリを落とす)。
        // 位置の許可が無ければ type location の startForeground が SecurityException になる
        try {
            startForeground(NOTIFICATION_ID, notification(), ServiceInfo.FOREGROUND_SERVICE_TYPE_LOCATION)
        } catch (_: SecurityException) {
            LocationTracking.denied()
            stopSelf()
            return START_NOT_STICKY
        } catch (_: RuntimeException) {
            LocationTracking.stop(TrackingStopReason.UNAVAILABLE)
            stopSelf()
            return START_NOT_STICKY
        }
        // 記録の相手(保存先)が居ない起動は止める
        if (!LocationTracking.state.value.isActive) {
            stopSelf()
            return START_NOT_STICKY
        }
        if (manager == null) {
            val lm = getSystemService(LOCATION_SERVICE) as? LocationManager
            val provider = lm?.let { provider(it) }
            if (lm == null || provider == null) {
                LocationTracking.stop(TrackingStopReason.UNAVAILABLE)
                stopSelf()
                return START_NOT_STICKY
            }
            try {
                lm.requestLocationUpdates(provider, UPDATE_MS, 0f, mainExecutor, listener)
            } catch (_: SecurityException) {
                LocationTracking.denied()
                stopSelf()
                return START_NOT_STICKY
            }
            manager = lm
            LocationTracking.began()
            handler.postDelayed(ticker, TICK_MS)
        }
        return START_NOT_STICKY
    }

    override fun onDestroy() {
        handler.removeCallbacks(ticker)
        manager?.removeUpdates(listener)
        manager = null
        // OS に止められた時も記録を止める(保存した点は残る)
        LocationTracking.stop()
        super.onDestroy()
    }

    /** 端末の融合した位置 → GPS の順(有効なものだけ)。 */
    private fun provider(lm: LocationManager): String? {
        val enabled = lm.getProviders(true)
        return listOf(LocationManager.FUSED_PROVIDER, LocationManager.GPS_PROVIDER).firstOrNull { it in enabled }
    }

    private fun notification(): Notification {
        val nm = getSystemService(NotificationManager::class.java)
        if (nm.getNotificationChannel(CHANNEL_ID) == null) {
            nm.createNotificationChannel(
                NotificationChannel(CHANNEL_ID, getString(R.string.renderer_tracker_channel), NotificationManager.IMPORTANCE_LOW),
            )
        }
        val stop = PendingIntent.getService(
            this,
            0,
            Intent(this, LocationTrackingService::class.java).setAction(ACTION_STOP),
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT,
        )
        val open = packageManager.getLaunchIntentForPackage(packageName)?.let {
            PendingIntent.getActivity(this, 0, it, PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT)
        }
        return NotificationCompat.Builder(this, CHANNEL_ID)
            .setSmallIcon(android.R.drawable.ic_menu_mylocation)
            .setContentTitle(getString(R.string.renderer_tracker_notification_title))
            .setContentText(getString(R.string.renderer_tracker_notification_text))
            .setOngoing(true)
            .setOnlyAlertOnce(true)
            .setCategory(NotificationCompat.CATEGORY_SERVICE)
            .setForegroundServiceBehavior(NotificationCompat.FOREGROUND_SERVICE_IMMEDIATE)
            .setContentIntent(open)
            .addAction(0, getString(R.string.renderer_tracker_stop_short), stop)
            .build()
    }

    companion object {
        const val ACTION_START = "jp.kz2601.metamon.renderer.location.START"
        const val ACTION_STOP = "jp.kz2601.metamon.renderer.location.STOP"
        private const val CHANNEL_ID = "location_tracking"
        private const val NOTIFICATION_ID = 0x7a7c
        private const val UPDATE_MS = 5_000L
        private const val TICK_MS = 1_000L
    }
}
