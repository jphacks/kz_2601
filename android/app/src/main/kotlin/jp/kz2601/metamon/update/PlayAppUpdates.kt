package jp.kz2601.metamon.update

import android.content.Context
import com.google.android.play.core.appupdate.AppUpdateInfo
import com.google.android.play.core.appupdate.AppUpdateManager
import com.google.android.play.core.appupdate.AppUpdateManagerFactory
import com.google.android.play.core.appupdate.AppUpdateOptions
import com.google.android.play.core.install.InstallStateUpdatedListener
import com.google.android.play.core.install.model.AppUpdateType
import com.google.android.play.core.install.model.InstallStatus
import com.google.android.play.core.install.model.UpdateAvailability
import com.google.android.play.core.ktx.requestAppUpdateInfo

/** Play の AppUpdateManager を [PlayUpdates] にする。 */
class PlayAppUpdates(private val manager: AppUpdateManager) : PlayUpdates {
    override suspend fun status(): PlayUpdateStatus = manager.requestAppUpdateInfo().toStatus()

    // AppUpdateInfo は 1 回しか使えないので、出す直前に読み直す
    override suspend fun start(flow: InAppUpdateFlow, launcher: UpdateLauncher): Boolean {
        val info = manager.requestAppUpdateInfo()
        val options = AppUpdateOptions.defaultOptions(flow.type)
        if (!info.isUpdateTypeAllowed(options)) return false
        return runCatching { manager.startUpdateFlowForResult(info, launcher, options) }.getOrDefault(false)
    }

    override fun completeUpdate() {
        manager.completeUpdate()
    }

    override fun listen(onChange: (FlexibleProgress) -> Unit): () -> Unit {
        val listener = InstallStateUpdatedListener { onChange(progressOf(it.installStatus())) }
        manager.registerListener(listener)
        return { manager.unregisterListener(listener) }
    }

    companion object {
        private const val PLAY_STORE = "com.android.vending"

        /** Play から入れた時だけ。stg・debug(adb・Firebase App Distribution)は null でストアへのリンクに落とす。 */
        fun ifInstalledFromPlay(context: Context): PlayAppUpdates? {
            val installer = runCatching { context.packageManager.getInstallSourceInfo(context.packageName).installingPackageName }.getOrNull()
            if (installer != PLAY_STORE) return null
            return PlayAppUpdates(AppUpdateManagerFactory.create(context))
        }
    }
}

private val InAppUpdateFlow.type: Int
    get() = when (this) {
        InAppUpdateFlow.IMMEDIATE -> AppUpdateType.IMMEDIATE
        InAppUpdateFlow.FLEXIBLE -> AppUpdateType.FLEXIBLE
    }

internal fun AppUpdateInfo.toStatus() = PlayUpdateStatus(
    availability = when (updateAvailability()) {
        UpdateAvailability.UPDATE_AVAILABLE -> PlayUpdateStatus.Availability.AVAILABLE
        UpdateAvailability.DEVELOPER_TRIGGERED_UPDATE_IN_PROGRESS -> PlayUpdateStatus.Availability.IN_PROGRESS
        else -> PlayUpdateStatus.Availability.NOT_AVAILABLE
    },
    immediateAllowed = isUpdateTypeAllowed(AppUpdateType.IMMEDIATE),
    flexibleAllowed = isUpdateTypeAllowed(AppUpdateType.FLEXIBLE),
    install = progressOf(installStatus()),
)

internal fun progressOf(installStatus: Int): FlexibleProgress = when (installStatus) {
    InstallStatus.PENDING, InstallStatus.DOWNLOADING -> FlexibleProgress.DOWNLOADING
    InstallStatus.DOWNLOADED -> FlexibleProgress.DOWNLOADED
    else -> FlexibleProgress.IDLE
}
