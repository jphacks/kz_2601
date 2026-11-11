package jp.kz2601.metamon.switcher

import androidx.compose.foundation.Image
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.MoreVert
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.R
import jp.kz2601.metamon.release.ReleaseStatusBadge
import jp.kz2601.metamon.services.AppSummary
import jp.kz2601.metamon.services.release.ReleaseRequest

/**
 * アプリの切り替え(iOS の AppSwitcherSheet の最小版・#881)。名前の一覧をタップで切り替え、「+ 新しいアプリ」で初期画面へ。
 * 各行の「…」メニュー(iOS の `AppCardActions`)から共有シート(#901)・リリース申請(#1254)。申請済みなら状態のバッジを出す。
 * 変身の後に撮った画面があれば行の左にサムネを出す(#1199・iOS の `AppThumbnailStore`)。無ければ名前だけ。
 * 並べ替え・名前の変更・削除は子 Issue。
 */
@Composable
fun AppSwitcherDialog(
    apps: List<AppSummary>,
    currentAppId: String,
    onSelect: (String) -> Unit,
    onNewApp: () -> Unit,
    onDismiss: () -> Unit,
    onShare: ((AppSummary) -> Unit)? = null,
    onRelease: ((AppSummary) -> Unit)? = null,
    releases: Map<String, ReleaseRequest> = emptyMap(),
    thumbnails: AppThumbnailStore? = null,
) {
    // 撮り直したら描き直す
    val thumbRevision = thumbnails?.revision?.collectAsState()?.value
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.switcher_title)) },
        text = {
            if (apps.isEmpty()) {
                Text(stringResource(R.string.switcher_empty))
            } else {
                LazyColumn(Modifier.heightIn(max = 360.dp)) {
                    items(apps, key = { it.id }) { app ->
                        Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                            val thumb = remember(app.id, thumbRevision) { thumbnails?.image(app.id) }
                            if (thumb != null) Thumbnail(thumb, app.id)
                            Column(
                                Modifier.weight(1f).clickable { onSelect(app.id) }.padding(vertical = 12.dp),
                            ) {
                                Text(app.name.ifEmpty { stringResource(R.string.switcher_untitled) }, style = MaterialTheme.typography.bodyLarge)
                                if (app.id == currentAppId) {
                                    Text(stringResource(R.string.switcher_current), style = MaterialTheme.typography.labelSmall)
                                }
                            }
                            Column(horizontalAlignment = Alignment.End) {
                                // 「…」: シェア・公開(#901)/ リリース申請(#1254)。どちらも使えない構成では出さない
                                if (onShare != null || onRelease != null) CardMenu(app, onShare, onRelease)
                                releases[app.id]?.let { ReleaseStatusBadge(it.kind) }
                            }
                        }
                    }
                }
            }
        },
        confirmButton = { TextButton(onClick = onNewApp) { Text(stringResource(R.string.switcher_new_app)) } },
        dismissButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.switcher_close)) } },
    )
}

/** カードのサムネ(縦長の画面を小さく・飾りなので読み上げない)。 */
@Composable
private fun Thumbnail(image: ImageBitmap, appId: String) {
    Image(
        image,
        contentDescription = null,
        contentScale = ContentScale.Crop,
        alignment = Alignment.TopCenter,
        modifier = Modifier
            .padding(end = 12.dp)
            .size(width = 44.dp, height = 72.dp)
            .clip(RoundedCornerShape(8.dp))
            .border(1.dp, MaterialTheme.colorScheme.outlineVariant, RoundedCornerShape(8.dp))
            .testTag("switcher.thumbnail.$appId"),
    )
}

@Composable
private fun CardMenu(app: AppSummary, onShare: ((AppSummary) -> Unit)?, onRelease: ((AppSummary) -> Unit)?) {
    var open by remember { mutableStateOf(false) }
    val name = app.name.ifEmpty { stringResource(R.string.switcher_untitled) }
    IconButton(onClick = { open = true }, modifier = Modifier.testTag("switcher.cardMenu.${app.id}")) {
        Icon(Icons.Filled.MoreVert, contentDescription = stringResource(R.string.release_menu, name))
    }
    DropdownMenu(expanded = open, onDismissRequest = { open = false }) {
        if (onShare != null) {
            DropdownMenuItem(
                text = { Text(stringResource(R.string.release_menu_share)) },
                onClick = {
                    open = false
                    onShare(app)
                },
                modifier = Modifier.testTag("switcher.share.${app.id}"),
            )
        }
        if (onRelease != null) {
            DropdownMenuItem(
                text = { Text(stringResource(R.string.release_menu_request)) },
                onClick = {
                    open = false
                    onRelease(app)
                },
                modifier = Modifier.testTag("switcher.release.${app.id}"),
            )
        }
    }
}
