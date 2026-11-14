package jp.kz2601.metamon.desktop.create

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.key
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.DpSize
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Window
import androidx.compose.ui.window.rememberWindowState
import jp.kz2601.metamon.desktop.DesktopApp
import jp.kz2601.metamon.desktop.LoadingSkeleton
import jp.kz2601.metamon.desktop.LocalTray
import jp.kz2601.metamon.desktop.chat.ChatErrorCopy
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.desktop.theme.TailorItTheme

/** 作っている画面の見出しの文(段階ごと)。 */
object CreateText {
    fun status(state: CreateState): String = when (state.phase) {
        CreatePhase.BUILDING -> state.preview?.app?.name?.ifBlank { null }?.let { Strings.format("voice_progress_app", it) } ?: Strings["voice_gen_building"]
        CreatePhase.DONE -> Strings["desktop_create_done"]
        CreatePhase.FAILED -> state.failure?.let { ChatErrorCopy.of(it).title } ?: Strings["voice_failed_title"]
        CreatePhase.CUT -> Strings["voice_offline_title"]
    }

    fun body(state: CreateState): String? = when (state.phase) {
        CreatePhase.FAILED -> state.failure?.let { ChatErrorCopy.of(it).body() }
        CreatePhase.CUT -> Strings["desktop_create_cut_body"]
        else -> null
    }
}

/**
 * 作っている間のウィンドウ(#2163)。届いた途中の DSL をそのまま DesktopApp で描き、完成したら閉じてそのアプリのウィンドウを開く。
 * 失敗・切れた時は案内と「もう一度」。作っている間の × ・「閉じる」は確認を出す。
 */
@Composable
fun CreateWindow(creator: AppCreator, creation: AppCreation) {
    val state by creation.state.collectAsState()
    Window(
        onCloseRequest = creator::requestClose,
        title = Strings["desktop_create_title"],
        state = rememberWindowState(size = DpSize(480.dp, 820.dp)),
    ) {
        TailorItTheme {
            Surface(Modifier.fillMaxSize()) {
                Column(Modifier.fillMaxSize()) {
                    Column(Modifier.fillMaxWidth().padding(12.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            Text(
                                CreateText.status(state),
                                style = MaterialTheme.typography.titleMedium,
                                modifier = Modifier.weight(1f).semantics { liveRegion = LiveRegionMode.Polite },
                            )
                            if (state.canRetry) Button(onClick = { creator.retry() }) { Text(Strings["voice_retry"]) }
                            TextButton(onClick = creator::requestClose) { Text(Strings["desktop_close"]) }
                        }
                        if (state.isBuilding) LinearProgressIndicator(Modifier.fillMaxWidth())
                        CreateText.body(state)?.let { Text(it, style = MaterialTheme.typography.bodyMedium) }
                    }
                    HorizontalDivider()
                    Box(Modifier.weight(1f)) {
                        val preview = state.preview
                        if (preview == null) {
                            LoadingSkeleton()
                        } else {
                            // 途中の画面はトレイのすぐ記録の行き先にしない。値が変わったら描き直す
                            CompositionLocalProvider(LocalTray provides null) {
                                key(preview) { DesktopApp(preview) }
                            }
                        }
                    }
                }
            }
            if (creator.confirmingClose) {
                AlertDialog(
                    onDismissRequest = creator::keepBuilding,
                    title = { Text(Strings["desktop_create_stop_title"]) },
                    text = { Text(Strings["desktop_create_stop_body"]) },
                    confirmButton = { TextButton(onClick = creator::stop) { Text(Strings["voice_gen_stop"]) } },
                    dismissButton = { TextButton(onClick = creator::keepBuilding) { Text(Strings["desktop_create_continue"]) } },
                )
            }
        }
    }
}
