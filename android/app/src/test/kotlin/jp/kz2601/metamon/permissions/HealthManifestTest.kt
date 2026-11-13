package jp.kz2601.metamon.permissions

import jp.kz2601.metamon.services.permissions.AppPermission
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * ヘルスコネクト(#870)の宣言がマニフェストにあること。宣言が無いと [PermissionCenter] は「使えない」扱いにし、
 * Health Connect も権限を聞かない。理由の画面(13 以前の action と 14+ の別名)も要る。
 */
class HealthManifestTest {
    private val manifest = File(File(System.getProperty("user.dir")), "src/main/AndroidManifest.xml").readText()

    @Test
    fun healthPermissionsAreDeclared() {
        for (name in AppPermission.HEALTH.manifestPermissions(AppPermission.TIRAMISU)) {
            assertTrue(name, manifest.contains("android:name=\"$name\""))
        }
    }

    @Test
    fun theRationaleScreenIsReachableFromBothWays() {
        assertTrue("13 以前", manifest.contains("androidx.health.ACTION_SHOW_PERMISSIONS_RATIONALE"))
        assertTrue("14+", manifest.contains("android.intent.action.VIEW_PERMISSION_USAGE"))
        assertTrue("14+ の category", manifest.contains("android.intent.category.HEALTH_PERMISSIONS"))
        assertTrue("別名は OS だけが開ける", manifest.contains("android.permission.START_VIEW_PERMISSION_USAGE"))
    }

    /** 13 以前は Health Connect のアプリが入っているかを見るので、queries が要る。 */
    @Test
    fun theHealthConnectAppIsVisible() {
        assertTrue(manifest.contains("com.google.android.apps.healthdata"))
    }
}
