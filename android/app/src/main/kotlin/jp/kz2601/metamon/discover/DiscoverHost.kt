package jp.kz2601.metamon.discover

import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import jp.kz2601.metamon.AppContainer
import jp.kz2601.metamon.R
import jp.kz2601.metamon.runtime.AppRuntime
import jp.kz2601.metamon.runtime.UiText
import jp.kz2601.metamon.services.InstallResult
import jp.kz2601.metamon.services.discover.DiscoverInstallResult
import jp.kz2601.metamon.services.discover.DiscoverModel
import kotlinx.coroutines.launch

/**
 * 「探す」のシートを出し、入れたアプリに切り替える(iOS の `SharingHooks` の showDiscover と `AppRuntime.installPublic`)。
 * インストールはシートを閉じてから進める(切り替えのトーストを見せる・iOS と同じ)。
 */
@Composable
fun DiscoverHost(container: AppContainer, open: Boolean, onDismiss: () -> Unit) {
    val runtime = container.runtime
    // 入れる・切り替えはシートを閉じた後も続くので、ここの scope で走らせる
    val scope = rememberCoroutineScope()
    val services = container.discover
    val model = remember(services) { DiscoverModel(services?.client, services?.moderation) }
    if (!open) return
    DiscoverSheet(
        model = model,
        onInstall = { item ->
            onDismiss()
            scope.launch {
                when (val result = model.install(item)) {
                    is DiscoverInstallResult.Installed -> adopt(runtime, result.app)
                    is DiscoverInstallResult.Failed -> runtime.showToast(UiText.Res(result.reason.installTextRes))
                }
            }
        },
        onDismiss = onDismiss,
    )
}

/** 入れたアプリに切り替えて一言出す(共有データ付きは「みんなの投稿も見られます」を添える)。 */
private suspend fun adopt(runtime: AppRuntime, app: InstallResult) {
    if (!runtime.switchApp(app.id)) return // 読めなければ切り替え側がトーストを出す
    val name: Any = app.name.ifEmpty { null } ?: UiText.Res(R.string.share_untitled)
    val head = UiText.Res(R.string.discover_installed, name)
    val extra: Any = when {
        app.member -> UiText.Res(R.string.share_open_extra_members)
        app.joined -> UiText.Res(R.string.share_open_extra_public)
        else -> ""
    }
    runtime.showToast(UiText.Res(R.string.share_open_message, head, extra))
    runtime.appsList()
}
