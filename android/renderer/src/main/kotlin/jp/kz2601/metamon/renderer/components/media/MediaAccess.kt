package jp.kz2601.metamon.renderer.components.media

import android.Manifest
import android.content.Context
import android.content.pm.PackageManager
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.platform.LocalContext

/** 使う直前に権限を聞く関数を返す Composable。`onResult(true)` で使ってよい。 */
typealias PermissionPrompt = @Composable (onResult: (Boolean) -> Unit) -> () -> Unit

/**
 * カメラの権限を聞く窓口。:app が権限の窓口(PermissionCenter・理由 → OS のダイアログ → 設定へ)で差し替える。
 * 既定は OS のダイアログだけ(プレビュー・テスト用)。:renderer は :app を知らないのでここで受け取る。
 */
val LocalCameraPermission = staticCompositionLocalOf<PermissionPrompt> { { onResult -> rememberPlainCameraPrompt(onResult) } }

@Composable
private fun rememberPlainCameraPrompt(onResult: (Boolean) -> Unit): () -> Unit {
    val context = LocalContext.current
    val callback by rememberUpdatedState(onResult)
    val launcher = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { callback(it) }
    return {
        if (context.checkSelfPermission(Manifest.permission.CAMERA) == PackageManager.PERMISSION_GRANTED) callback(true)
        else launcher.launch(Manifest.permission.CAMERA)
    }
}

/** 端末にカメラがあるか(無ければ写真選択に落とす)。 */
fun Context.hasCamera(): Boolean = packageManager.hasSystemFeature(PackageManager.FEATURE_CAMERA_ANY)
