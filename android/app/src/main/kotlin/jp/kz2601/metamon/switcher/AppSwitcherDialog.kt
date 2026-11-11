package jp.kz2601.metamon.switcher

import androidx.annotation.StringRes
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
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.MoreVert
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
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
 * 「…」の名前の変更・複製・左右へ移動・削除(#896・iOS の `AppManageMenuItems`)。削除は確認してから。
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
    edits: AppSwitcherEdits? = null,
) {
    var renaming by remember { mutableStateOf<AppSummary?>(null) }
    var deleting by remember { mutableStateOf<AppSummary?>(null) }
    val shown = if (edits == null || edits.hidden.isEmpty()) apps else apps.filter { it.id !in edits.hidden }
    // 撮り直したら描き直す
    val thumbRevision = thumbnails?.revision?.collectAsState()?.value
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.switcher_title)) },
        text = {
            if (shown.isEmpty()) {
                Text(stringResource(R.string.switcher_empty))
            } else {
                LazyColumn(Modifier.heightIn(max = 360.dp)) {
                    itemsIndexed(shown, key = { _, app -> app.id }) { index, app ->
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
                                if (onShare != null || onRelease != null || edits != null) {
                                    CardMenu(
                                        app,
                                        onShare,
                                        onRelease,
                                        edits?.let { e ->
                                            CardEdits(
                                                index = index,
                                                count = shown.size,
                                                onRename = { renaming = app },
                                                onDuplicate = { e.onDuplicate(app) },
                                                onMove = { to -> e.onMove(app, to) },
                                                onDelete = { deleting = app },
                                            )
                                        },
                                    )
                                }
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
    if (edits != null) {
        renaming?.let { app ->
            RenameDialog(app, onDismiss = { renaming = null }) { name ->
                renaming = null
                edits.onRename(app, name)
            }
        }
        deleting?.let { app ->
            DeleteConfirm(app, isCurrent = app.id == currentAppId, canDelete = edits.canDelete, onDismiss = { deleting = null }) {
                deleting = null
                edits.onDelete(app)
            }
        }
    }
}

/** 「…」の整理の操作(#896)。呼ぶ側が [AppListEditor] につなぐ。 */
class AppSwitcherEdits(
    val onRename: (AppSummary, String) -> Unit,
    val onDuplicate: (AppSummary) -> Unit,
    /** 一覧の何番目に動かすか */
    val onMove: (AppSummary, Int) -> Unit,
    val onDelete: (AppSummary) -> Unit,
    /** 削除中で隠しているアプリ(楽観的 UI) */
    val hidden: Set<String> = emptySet(),
    /** 生成中は削除できない */
    val canDelete: Boolean = true,
)

/** 1 枚のカードの整理の操作。 */
private class CardEdits(
    val index: Int,
    val count: Int,
    val onRename: () -> Unit,
    val onDuplicate: () -> Unit,
    val onMove: (Int) -> Unit,
    val onDelete: () -> Unit,
)

@Composable
private fun RenameDialog(app: AppSummary, onDismiss: () -> Unit, onConfirm: (String) -> Unit) {
    var text by remember(app.id) { mutableStateOf(app.name) }
    val valid = AppListEdit.normalizedName(text) != null
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.app_menu_rename_title)) },
        text = {
            Column {
                Text(stringResource(R.string.app_menu_rename_message, AppListEdit.MAX_NAME_LENGTH), style = MaterialTheme.typography.bodyMedium)
                OutlinedTextField(
                    value = text,
                    onValueChange = { text = it },
                    singleLine = true,
                    label = { Text(stringResource(R.string.app_menu_rename_field)) },
                    isError = !valid,
                    modifier = Modifier.fillMaxWidth().padding(top = 12.dp).testTag("switcher.rename.field"),
                )
            }
        },
        confirmButton = {
            TextButton(onClick = { onConfirm(text) }, enabled = valid, modifier = Modifier.testTag("switcher.rename.confirm")) {
                Text(stringResource(R.string.app_menu_rename_confirm))
            }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.app_menu_cancel)) } },
    )
}

@Composable
private fun DeleteConfirm(app: AppSummary, isCurrent: Boolean, canDelete: Boolean, onDismiss: () -> Unit, onConfirm: () -> Unit) {
    val name = app.name.ifEmpty { stringResource(R.string.switcher_untitled) }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.app_menu_delete_title, name)) },
        text = {
            val message = when {
                !canDelete -> R.string.app_menu_delete_busy
                isCurrent -> R.string.app_menu_delete_message_current
                else -> R.string.app_menu_delete_message
            }
            Text(stringResource(message))
        },
        confirmButton = {
            TextButton(
                onClick = onConfirm,
                enabled = canDelete,
                colors = ButtonDefaults.textButtonColors(contentColor = MaterialTheme.colorScheme.error),
                modifier = Modifier.testTag("switcher.delete.confirm"),
            ) { Text(stringResource(R.string.app_menu_delete)) }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.app_menu_cancel)) } },
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
private fun CardMenu(app: AppSummary, onShare: ((AppSummary) -> Unit)?, onRelease: ((AppSummary) -> Unit)?, edits: CardEdits?) {
    var open by remember { mutableStateOf(false) }
    val name = app.name.ifEmpty { stringResource(R.string.switcher_untitled) }
    IconButton(onClick = { open = true }, modifier = Modifier.testTag("switcher.cardMenu.${app.id}")) {
        Icon(Icons.Filled.MoreVert, contentDescription = stringResource(R.string.release_menu, name))
    }
    DropdownMenu(expanded = open, onDismissRequest = { open = false }) {
        val close = { open = false }
        if (edits != null) {
            MenuEntry(R.string.app_menu_rename, "switcher.rename.${app.id}", close, action = edits.onRename)
            MenuEntry(R.string.app_menu_duplicate, "switcher.duplicate.${app.id}", close, action = edits.onDuplicate)
            if (edits.count > 1) {
                MenuEntry(R.string.app_menu_move_left, "switcher.moveLeft.${app.id}", close, enabled = edits.index > 0) { edits.onMove(edits.index - 1) }
                MenuEntry(R.string.app_menu_move_right, "switcher.moveRight.${app.id}", close, enabled = edits.index < edits.count - 1) {
                    edits.onMove(edits.index + 1)
                }
            }
            HorizontalDivider()
        }
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
        if (edits != null) MenuEntry(R.string.app_menu_delete, "switcher.delete.${app.id}", close, action = edits.onDelete)
    }
}

@Composable
private fun MenuEntry(@StringRes label: Int, tag: String, close: () -> Unit, enabled: Boolean = true, action: () -> Unit) {
    DropdownMenuItem(
        text = { Text(stringResource(label)) },
        onClick = {
            close()
            action()
        },
        enabled = enabled,
        modifier = Modifier.testTag(tag),
    )
}
