package jp.kz2601.metamon.services.permissions

import jp.kz2601.metamon.services.permissions.PermissionStatus.DENIED
import jp.kz2601.metamon.services.permissions.PermissionStatus.GRANTED
import jp.kz2601.metamon.services.permissions.PermissionStatus.NOT_ASKED
import jp.kz2601.metamon.services.permissions.PermissionStatus.PERMANENTLY_DENIED
import jp.kz2601.metamon.services.permissions.PermissionStatus.UNAVAILABLE
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class PermissionRulesTest {
    @Test
    fun 一度も聞いていなければ未確認() {
        assertEquals(NOT_ASKED, PermissionRules.resolve(granted = false, stored = null, shouldShowRationale = false))
    }

    @Test
    fun 許可されていれば前の結果に関係なく許可() {
        for (stored in listOf(null, NOT_ASKED, DENIED, PERMANENTLY_DENIED)) {
            assertEquals(GRANTED, PermissionRules.resolve(granted = true, stored = stored, shouldShowRationale = false))
        }
    }

    @Test
    fun 理由を出すべき時は拒否でまだ聞ける() {
        assertEquals(DENIED, PermissionRules.resolve(granted = false, stored = null, shouldShowRationale = true))
        assertEquals(DENIED, PermissionRules.resolve(granted = false, stored = PERMANENTLY_DENIED, shouldShowRationale = true))
    }

    @Test
    fun 二度と聞かないを覚えていればそのまま() {
        assertEquals(PERMANENTLY_DENIED, PermissionRules.resolve(granted = false, stored = PERMANENTLY_DENIED, shouldShowRationale = false))
    }

    @Test
    fun 設定で外された時はまだ聞ける扱い() {
        assertEquals(DENIED, PermissionRules.resolve(granted = false, stored = GRANTED, shouldShowRationale = false))
    }

    @Test
    fun 一回目に断られたら拒否() {
        assertEquals(DENIED, PermissionRules.afterResult(NOT_ASKED, granted = false, shouldShowRationale = true))
    }

    @Test
    fun 一回目にダイアログの外で閉じたらまだ聞ける() {
        assertEquals(DENIED, PermissionRules.afterResult(NOT_ASKED, granted = false, shouldShowRationale = false))
    }

    @Test
    fun 拒否の後にまた断られて理由も出ないなら二度と聞かない() {
        assertEquals(PERMANENTLY_DENIED, PermissionRules.afterResult(DENIED, granted = false, shouldShowRationale = false))
    }

    @Test
    fun 許可されたら許可() {
        assertEquals(GRANTED, PermissionRules.afterResult(DENIED, granted = true, shouldShowRationale = false))
        assertEquals(GRANTED, PermissionRules.afterResult(PERMANENTLY_DENIED, granted = true, shouldShowRationale = false))
    }

    @Test
    fun 二度と聞かない時だけ設定へ誘導する() {
        assertEquals(PermissionStep.PROCEED, PermissionRules.nextStep(GRANTED))
        assertEquals(PermissionStep.EXPLAIN_THEN_ASK, PermissionRules.nextStep(NOT_ASKED))
        assertEquals(PermissionStep.EXPLAIN_THEN_ASK, PermissionRules.nextStep(DENIED))
        assertEquals(PermissionStep.OPEN_SETTINGS, PermissionRules.nextStep(PERMANENTLY_DENIED))
        assertEquals(PermissionStep.UNAVAILABLE, PermissionRules.nextStep(UNAVAILABLE))
    }

    @Test
    fun オフは拒否と二度と聞かないだけ() {
        assertTrue(PermissionRules.isOff(DENIED))
        assertTrue(PermissionRules.isOff(PERMANENTLY_DENIED))
        assertFalse(PermissionRules.isOff(NOT_ASKED))
        assertFalse(PermissionRules.isOff(GRANTED))
        assertFalse(PermissionRules.isOff(UNAVAILABLE))
    }

    @Test
    fun 通知は12では設定のオンオフだけ() {
        assertEquals(GRANTED, PermissionRules.resolveNotifications(32, enabled = true) { error("聞かない") })
        assertEquals(PERMANENTLY_DENIED, PermissionRules.resolveNotifications(32, enabled = false) { error("聞かない") })
    }

    @Test
    fun 通知は13以上で許可済みでも設定で切られていれば設定へ() {
        assertEquals(PERMANENTLY_DENIED, PermissionRules.resolveNotifications(33, enabled = false) { GRANTED })
        assertEquals(GRANTED, PermissionRules.resolveNotifications(33, enabled = true) { GRANTED })
        assertEquals(NOT_ASKED, PermissionRules.resolveNotifications(33, enabled = false) { NOT_ASKED })
    }

    @Test
    fun 版ごとの権限名() {
        assertEquals(emptyList<String>(), AppPermission.NOTIFICATIONS.manifestPermissions(32))
        assertEquals(listOf(AppPermission.POST_NOTIFICATIONS), AppPermission.NOTIFICATIONS.manifestPermissions(33))
    }

    @Test
    fun 位置はおおよそだけでも許可() {
        val coarseOnly = mapOf(AppPermission.ACCESS_COARSE_LOCATION to true, AppPermission.ACCESS_FINE_LOCATION to false)
        assertTrue(AppPermission.LOCATION.isGranted(coarseOnly, 34))
        assertFalse(AppPermission.LOCATION.isGranted(emptyMap(), 34))
    }

    @Test
    fun 近距離の権限は持たない() {
        // 近距離共有(#878)を入れるまで、Bluetooth・Wi-Fi の近くの端末を求める単位は無い
        val nearby = setOf(
            "android.permission.BLUETOOTH_SCAN",
            "android.permission.BLUETOOTH_ADVERTISE",
            "android.permission.BLUETOOTH_CONNECT",
            "android.permission.NEARBY_WIFI_DEVICES",
        )
        for (sdk in listOf(31, 33, 35)) {
            assertTrue(AppPermission.entries.flatMap { it.manifestPermissions(sdk) }.none { it in nearby })
        }
    }
}
