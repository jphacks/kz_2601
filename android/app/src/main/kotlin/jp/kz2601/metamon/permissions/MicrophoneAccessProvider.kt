package jp.kz2601.metamon.permissions

import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import jp.kz2601.metamon.renderer.components.audio.LocalMicrophoneAccess
import jp.kz2601.metamon.renderer.components.audio.MicrophoneAccess
import jp.kz2601.metamon.services.permissions.AppPermission

/**
 * 描画器の音の部品(audioRecorder・transcribeField・#864)に、マイクの権限を権限の窓口(#890)で聞く口を渡す。
 * 理由 → OS のダイアログ、断られた後は「設定を開く」。同時に聞くのは 1 件(後から来た分は前の結果を待たずに置き換える)。
 */
@Composable
fun MicrophoneAccessProvider(content: @Composable () -> Unit) {
    val pending = remember { arrayOfNulls<(Boolean) -> Unit>(1) }
    val ask = rememberPermissionRequest(AppPermission.MICROPHONE) { granted ->
        val callback = pending[0]
        pending[0] = null
        callback?.invoke(granted)
    }
    val latestAsk by rememberUpdatedState(ask)
    val access = remember {
        MicrophoneAccess { onResult ->
            pending[0] = onResult
            latestAsk()
        }
    }
    CompositionLocalProvider(LocalMicrophoneAccess provides access, content = content)
}
