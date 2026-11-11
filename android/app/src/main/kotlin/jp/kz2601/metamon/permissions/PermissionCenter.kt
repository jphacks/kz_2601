package jp.kz2601.metamon.permissions

import android.annotation.SuppressLint
import android.app.Activity
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Build
import android.provider.Settings
import androidx.core.app.NotificationManagerCompat
import jp.kz2601.metamon.services.permissions.AppPermission
import jp.kz2601.metamon.services.permissions.PermissionRules
import jp.kz2601.metamon.services.permissions.PermissionStatus
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

/**
 * 権限の窓口(#890)。権限ごとの状態(未確認・許可・拒否・二度と聞かない)を持ち、聞いた結果を覚える。
 * 判定の中身は [PermissionRules](JVM テスト)。Compose からは [rememberPermissionRequest] で聞く。
 */
class PermissionCenter private constructor(private val app: Context) {
    private val prefs = app.getSharedPreferences(PREFS, Context.MODE_PRIVATE)
    private val _statuses = MutableStateFlow<Map<AppPermission, PermissionStatus>>(emptyMap())

    /** 全権限の今の状態。画面に戻った時([refresh])に取り直す。 */
    val statuses: StateFlow<Map<AppPermission, PermissionStatus>> = _statuses.asStateFlow()

    private val declared: Set<String> by lazy {
        val pm = app.packageManager
        runCatching {
            requestedPermissions(
                SDK,
                // 13+ の時だけ呼ばれる([requestedPermissions] が版を見る。テストのため版を引数で渡すので lint が追えない)
                modern = @SuppressLint("NewApi") {
                    pm.getPackageInfo(app.packageName, PackageManager.PackageInfoFlags.of(PackageManager.GET_PERMISSIONS.toLong())).requestedPermissions
                },
                legacy = {
                    @Suppress("DEPRECATION")
                    pm.getPackageInfo(app.packageName, PackageManager.GET_PERMISSIONS).requestedPermissions
                },
            )
        }.getOrNull().orEmpty()
    }

    /** [activity] は「理由を出すべきか」を見るのに要る。無ければ前に覚えた結果だけで決める。 */
    fun refresh(activity: Activity?) {
        _statuses.value = AppPermission.entries.associateWith { status(it, activity) }
    }

    fun status(permission: AppPermission, activity: Activity?): PermissionStatus {
        val names = permission.manifestPermissions(SDK)
        if (permission == AppPermission.NOTIFICATIONS) {
            val enabled = NotificationManagerCompat.from(app).areNotificationsEnabled()
            return PermissionRules.resolveNotifications(SDK, enabled) { runtimeStatus(permission, names, activity) }
        }
        // マニフェストに無い権限は聞けない(その部品がまだ無い)。バーも出さない
        if (names.isEmpty() || !declared.containsAll(names)) return PermissionStatus.UNAVAILABLE
        return runtimeStatus(permission, names, activity)
    }

    /** OS のダイアログの結果を覚える。[before] は聞く前の状態。 */
    fun record(permission: AppPermission, results: Map<String, Boolean>, before: PermissionStatus, activity: Activity?): PermissionStatus {
        val names = permission.manifestPermissions(SDK)
        val granted = permission.isGranted(names.associateWith { results[it] == true || isGranted(it) }, SDK)
        val after = PermissionRules.afterResult(before, granted, shouldShowRationale(names, activity))
        prefs.edit().putString(permission.name, after.name).apply()
        refresh(activity)
        return status(permission, activity)
    }

    /** ログアウト・アカウント削除(#841): 覚えた結果を忘れる(OS の許可そのものは OS の設定のまま)。 */
    fun forget() {
        prefs.edit().clear().apply()
        refresh(null)
    }

    /** OS のダイアログで聞く権限名(この版の分)。 */
    fun requestNames(permission: AppPermission): Array<String> = permission.manifestPermissions(SDK).toTypedArray()

    /** 「設定を開く」の行き先。通知は通知の設定、ヘルスコネクトはその設定、ほかはアプリの情報。 */
    fun settingsIntent(permission: AppPermission): Intent = when (permission) {
        AppPermission.NOTIFICATIONS -> Intent(Settings.ACTION_APP_NOTIFICATION_SETTINGS)
            .putExtra(Settings.EXTRA_APP_PACKAGE, app.packageName)
        else -> appDetailsIntent()
    }.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)

    /** 設定を開く。行き先が無ければアプリの情報に落とす。 */
    fun openSettings(context: Context, permission: AppPermission) {
        runCatching { context.startActivity(settingsIntent(permission)) }
            .onFailure { runCatching { context.startActivity(appDetailsIntent().addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)) } }
    }

    private fun appDetailsIntent() = Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS, Uri.fromParts("package", app.packageName, null))

    private fun runtimeStatus(permission: AppPermission, names: List<String>, activity: Activity?): PermissionStatus {
        val granted = permission.isGranted(names.associateWith(::isGranted), SDK)
        val stored = prefs.getString(permission.name, null)?.let { runCatching { PermissionStatus.valueOf(it) }.getOrNull() }
        return PermissionRules.resolve(granted, stored, shouldShowRationale(names, activity))
    }

    private fun isGranted(name: String) = app.checkSelfPermission(name) == PackageManager.PERMISSION_GRANTED

    private fun shouldShowRationale(names: List<String>, activity: Activity?) =
        activity != null && names.any { activity.shouldShowRequestPermissionRationale(it) }

    companion object {
        /**
         * マニフェストで宣言した権限。`PackageInfoFlags` は 13(API 33)から(minSdk 31 の 12 / 12L で呼ぶと
         * NoSuchMethodError になり、全部の権限が「使えない」扱いになる)ので、12 / 12L は int の版を使う。
         */
        internal fun requestedPermissions(sdkInt: Int, modern: () -> Array<String>?, legacy: () -> Array<String>?): Set<String> =
            (if (sdkInt >= AppPermission.TIRAMISU) modern() else legacy())?.toSet().orEmpty()

        private const val PREFS = "permissions"
        private val SDK = Build.VERSION.SDK_INT

        @Volatile private var instance: PermissionCenter? = null

        fun get(context: Context): PermissionCenter =
            instance ?: synchronized(this) { instance ?: PermissionCenter(context.applicationContext).also { instance = it } }
    }
}
