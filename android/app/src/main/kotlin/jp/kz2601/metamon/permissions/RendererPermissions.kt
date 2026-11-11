package jp.kz2601.metamon.permissions

import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import jp.kz2601.metamon.renderer.components.media.LocalCameraPermission
import jp.kz2601.metamon.services.permissions.AppPermission

/** 描画器の部品が聞く権限を、アプリの窓口(理由 → OS のダイアログ → 設定へ)に通す(#862)。 */
@Composable
fun ProvideRendererPermissions(content: @Composable () -> Unit) {
    CompositionLocalProvider(
        LocalCameraPermission provides { onResult -> rememberPermissionRequest(AppPermission.CAMERA, onResult = onResult) },
        content = content,
    )
}
