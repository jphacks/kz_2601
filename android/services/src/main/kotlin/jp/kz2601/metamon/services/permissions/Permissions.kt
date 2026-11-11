package jp.kz2601.metamon.services.permissions

/**
 * 実行時に聞く権限の単位(#890)。1 つの単位が OS の権限を複数まとめることがある(位置・近距離)。
 * OS の権限名は版で変わるので [manifestPermissions] で引く。
 */
enum class AppPermission {
    MICROPHONE,
    CAMERA,

    /** おおよそ / 正確。どちらか許可されていれば使える(おおよそだけでも許可扱い)。 */
    LOCATION,

    /** 13+ は POST_NOTIFICATIONS。12 / 12L は実行時の権限が無く、設定のオン・オフだけ。 */
    NOTIFICATIONS,

    /**
     * 近くの端末と送り合う(nearbyShare・Nearby Connections・#1380)。Bluetooth 3 つと、
     * 13+ は Wi-Fi の近くの端末・12 / 12L は正確な位置(Nearby Connections が 12L までは位置で見つける)。
     */
    NEARBY,

    /** Health Connect(歩数・体重)。 */
    HEALTH,
    ;

    /** この版で求める OS の権限。空なら実行時に聞く権限は無い。 */
    fun manifestPermissions(sdkInt: Int): List<String> = when (this) {
        MICROPHONE -> listOf(RECORD_AUDIO)
        CAMERA -> listOf(CAMERA_PERMISSION)
        LOCATION -> listOf(ACCESS_COARSE_LOCATION, ACCESS_FINE_LOCATION)
        NOTIFICATIONS -> if (sdkInt >= TIRAMISU) listOf(POST_NOTIFICATIONS) else emptyList()
        NEARBY -> listOf(BLUETOOTH_SCAN, BLUETOOTH_ADVERTISE, BLUETOOTH_CONNECT) +
            if (sdkInt >= TIRAMISU) listOf(NEARBY_WIFI_DEVICES) else listOf(ACCESS_FINE_LOCATION)
        HEALTH -> HEALTH_PERMISSIONS
    }

    /** 要求の結果(権限名 → 許可)から、この単位が使えるか。位置はどちらか 1 つで良い。それ以外は全部要る。 */
    fun isGranted(results: Map<String, Boolean>, sdkInt: Int): Boolean {
        val names = manifestPermissions(sdkInt)
        if (names.isEmpty()) return true
        return if (this == LOCATION) names.any { results[it] == true } else names.all { results[it] == true }
    }

    companion object {
        const val TIRAMISU = 33

        const val RECORD_AUDIO = "android.permission.RECORD_AUDIO"
        const val CAMERA_PERMISSION = "android.permission.CAMERA"
        const val ACCESS_COARSE_LOCATION = "android.permission.ACCESS_COARSE_LOCATION"
        const val ACCESS_FINE_LOCATION = "android.permission.ACCESS_FINE_LOCATION"
        const val POST_NOTIFICATIONS = "android.permission.POST_NOTIFICATIONS"
        const val BLUETOOTH_SCAN = "android.permission.BLUETOOTH_SCAN"
        const val BLUETOOTH_ADVERTISE = "android.permission.BLUETOOTH_ADVERTISE"
        const val BLUETOOTH_CONNECT = "android.permission.BLUETOOTH_CONNECT"
        const val NEARBY_WIFI_DEVICES = "android.permission.NEARBY_WIFI_DEVICES"
        val HEALTH_PERMISSIONS = listOf(
            "android.permission.health.READ_STEPS",
            "android.permission.health.READ_WEIGHT",
            "android.permission.health.WRITE_WEIGHT",
        )
    }
}

/** 権限の状態。[UNAVAILABLE] はマニフェストに無い・この端末では使えない(バーも出さない)。 */
enum class PermissionStatus { UNAVAILABLE, NOT_ASKED, GRANTED, DENIED, PERMANENTLY_DENIED }

/** 次にすること。 */
enum class PermissionStep {
    /** もう使える */
    PROCEED,

    /** 理由を先に出して OS のダイアログ */
    EXPLAIN_THEN_ASK,

    /** OS のダイアログはもう出ない。理由と「設定を開く」 */
    OPEN_SETTINGS,

    /** この端末では使えない */
    UNAVAILABLE,
}

/** 状態の移り変わり。Android の部品に触らない(JVM テスト)。 */
object PermissionRules {
    /**
     * 今の端末の様子から状態を決める。
     * - [stored]: 前に聞いた結果(一度も聞いていなければ null)
     * - [shouldShowRationale]: `shouldShowRequestPermissionRationale`。1 回断ると true、2 回断る(二度と聞かない)と false に戻る
     */
    fun resolve(granted: Boolean, stored: PermissionStatus?, shouldShowRationale: Boolean): PermissionStatus = when {
        granted -> PermissionStatus.GRANTED
        shouldShowRationale -> PermissionStatus.DENIED
        stored == null || stored == PermissionStatus.NOT_ASKED -> PermissionStatus.NOT_ASKED
        stored == PermissionStatus.PERMANENTLY_DENIED -> PermissionStatus.PERMANENTLY_DENIED
        // 一度聞いて断られた / 許可を設定で外された。OS がまた聞けるかは分からないので聞いてみる(すぐ断られたら [afterResult] で二度と聞かないへ)
        else -> PermissionStatus.DENIED
    }

    /**
     * OS のダイアログの結果を受けた後の状態。
     * 1 回目に断られても理由の表示(rationale)が false のことがある(ダイアログの外を押して閉じた)ので、その時はまだ聞ける扱い。
     * 断られた後にもう一度聞いて、rationale が false のままなら二度と聞かない。
     */
    fun afterResult(previous: PermissionStatus, granted: Boolean, shouldShowRationale: Boolean): PermissionStatus = when {
        granted -> PermissionStatus.GRANTED
        shouldShowRationale -> PermissionStatus.DENIED
        previous == PermissionStatus.DENIED || previous == PermissionStatus.PERMANENTLY_DENIED -> PermissionStatus.PERMANENTLY_DENIED
        else -> PermissionStatus.DENIED
    }

    fun nextStep(status: PermissionStatus): PermissionStep = when (status) {
        PermissionStatus.GRANTED -> PermissionStep.PROCEED
        PermissionStatus.NOT_ASKED, PermissionStatus.DENIED -> PermissionStep.EXPLAIN_THEN_ASK
        PermissionStatus.PERMANENTLY_DENIED -> PermissionStep.OPEN_SETTINGS
        PermissionStatus.UNAVAILABLE -> PermissionStep.UNAVAILABLE
    }

    /** 「オフ」のバーを出す状態(断られている)。 */
    fun isOff(status: PermissionStatus): Boolean =
        status == PermissionStatus.DENIED || status == PermissionStatus.PERMANENTLY_DENIED

    /**
     * 通知(12 / 12L は実行時の権限が無い・13+ でも設定で切れる)。[enabled] は NotificationManager の areNotificationsEnabled。
     * 設定で切られていればダイアログでは戻せないので二度と聞かない扱い。
     */
    fun resolveNotifications(sdkInt: Int, enabled: Boolean, permission: () -> PermissionStatus): PermissionStatus = when {
        sdkInt < AppPermission.TIRAMISU -> if (enabled) PermissionStatus.GRANTED else PermissionStatus.PERMANENTLY_DENIED
        else -> {
            val s = permission()
            if (s == PermissionStatus.GRANTED && !enabled) PermissionStatus.PERMANENTLY_DENIED else s
        }
    }
}
