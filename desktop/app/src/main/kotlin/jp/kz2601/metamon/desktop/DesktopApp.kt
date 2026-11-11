package jp.kz2601.metamon.desktop

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.FilterChip
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateMapOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.runtime.snapshots.SnapshotStateMap
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.input.VisualTransformation
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.Entity
import jp.kz2601.metamon.dsl.screen
import jp.kz2601.metamon.desktop.io.DesktopIo
import jp.kz2601.metamon.desktop.io.FileImportLeaf
import jp.kz2601.metamon.desktop.io.IoOverlay
import jp.kz2601.metamon.desktop.io.ListIoBar
import jp.kz2601.metamon.desktop.io.MediaFieldLeaf
import jp.kz2601.metamon.desktop.io.ioDropTarget
import jp.kz2601.metamon.desktop.io.previewKey
import jp.kz2601.metamon.desktop.io.rememberDesktopIo
import androidx.compose.ui.input.key.onPreviewKeyEvent
import java.awt.Desktop
import java.net.URI

private data class NavEntry(val screenId: String, val recordId: String?, val key: Int)

/** 画面 1 枚の描画に要るもの。 */
private class ScreenScope(
    val entity: Entity?,
    val draft: SnapshotStateMap<String, String>,
    val store: MockStore,
    val perform: (Action, String?) -> Unit,
    val entityOf: (String?) -> Entity?,
    val io: DesktopIo,
)

/** DSL を 1 本読み、スタックで画面を積んで描く(骨格: grid の配置はせず DSL の順に縦に積む)。 */
@Composable
fun DesktopApp(dsl: AppDSL) {
    var store by remember { mutableStateOf(MockStore()) }
    var stack by remember { mutableStateOf(listOf(NavEntry(dsl.initialScreen, null, 0))) }
    var nextKey by remember { mutableStateOf(1) }
    var message by remember { mutableStateOf<String?>(null) }
    val io = rememberDesktopIo(dsl, { store }, { store = it })

    val top = stack.last()
    val screen = dsl.screen(top.screenId) ?: dsl.screens.firstOrNull() ?: return
    val entityOf = { id: String? -> dsl.entities.firstOrNull { it.id == id } }
    val entity = entityOf(screen.entity)
    val draft = remember(top.key) {
        mutableStateMapOf<String, String>().apply {
            val saved = if (entity != null && top.recordId != null) store.find(entity.id, top.recordId) else null
            putAll(saved?.data ?: entity?.defaultDraft().orEmpty())
        }
    }

    fun pop() {
        if (stack.size > 1) stack = stack.dropLast(1)
    }

    fun perform(action: Action, recordId: String?) {
        message = null
        when (action.type) {
            ActionType.NAVIGATE -> action.screen?.let {
                stack = stack + NavEntry(it, recordId, nextKey)
                nextKey++
            }
            ActionType.BACK -> pop()
            ActionType.SAVE_RECORD -> {
                val target = entityOf(action.entity ?: screen.entity)
                if (target == null) {
                    message = "保存先のフォームがありません"
                    return
                }
                val missing = target.missingRequired(draft)
                if (missing.isNotEmpty()) {
                    message = "入力してください: ${missing.joinToString("、")}"
                    return
                }
                store = store.save(target.id, top.recordId, draft.toMap())
                pop()
            }
            ActionType.DELETE_RECORD -> {
                val target = entityOf(action.entity ?: screen.entity)
                if (target != null && top.recordId != null) store = store.delete(target.id, top.recordId)
                pop()
            }
            ActionType.OPEN_URL -> action.url?.let(::openBrowser)
            ActionType.EXPORT -> io.export(action, screen.entity)
            else -> message = "デスクトップ版ではまだ使えない操作: ${action.rawType.ifEmpty { action.type.name }}"
        }
    }

    val scope = ScreenScope(entity, draft, store, ::perform, entityOf, io)
    Surface(Modifier.fillMaxSize().ioDropTarget(io, { screen.components }, { draft }).onPreviewKeyEvent(io.previewKey({ screen.components }, { draft }))) {
        IoOverlay(io)
        Column(Modifier.fillMaxSize()) {
            Row(Modifier.fillMaxWidth().padding(8.dp), verticalAlignment = Alignment.CenterVertically) {
                if (stack.size > 1) TextButton(onClick = ::pop) { Text("戻る") }
                Text(screen.title.ifEmpty { dsl.app.name }, style = MaterialTheme.typography.titleLarge, modifier = Modifier.weight(1f))
                screen.toolbar.forEach { b -> TextButton(onClick = { perform(b.action, top.recordId) }) { Text(b.label) } }
            }
            HorizontalDivider()
            message?.let { Text(it, color = MaterialTheme.colorScheme.error, modifier = Modifier.padding(12.dp)) }
            io.notice?.let { Text(it, modifier = Modifier.padding(12.dp)) }
            Column(
                Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(16.dp),
                verticalArrangement = Arrangement.spacedBy(12.dp),
            ) {
                screen.components.forEach { Node(it, scope) }
            }
        }
    }
}

@Composable
private fun Node(c: Component, s: ScreenScope) {
    when (c.type) {
        ComponentType.HSTACK -> Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
            c.children.forEach { Box1(Modifier.weight(1f)) { Node(it, s) } }
        }
        ComponentType.CARD -> Card(Modifier.fillMaxWidth()) { Children(c, s, Modifier.padding(12.dp)) }
        in DesktopLeaves.containers -> Children(c, s)
        ComponentType.TEXT, ComponentType.MARKDOWN -> Text(c.text.orEmpty())
        ComponentType.LABEL, ComponentType.BADGE -> Text(c.label ?: c.text.orEmpty(), style = MaterialTheme.typography.labelLarge)
        ComponentType.EMPTY_STATE -> Text(c.title ?: c.message ?: c.text.orEmpty(), color = Color.Gray)
        ComponentType.SPACER -> Spacer(Modifier.height(8.dp))
        ComponentType.DIVIDER -> HorizontalDivider()
        ComponentType.BUTTON -> Button(onClick = { c.action?.let { s.perform(it, null) } }) { Text(c.label ?: c.title ?: c.text.orEmpty()) }
        ComponentType.LINK -> TextButton(onClick = { c.url?.let(::openBrowser) }) { Text(c.text ?: c.label ?: c.url.orEmpty()) }
        ComponentType.LIST -> ListLeaf(c, s)
        ComponentType.FILE_IMPORT -> FileImportLeaf(c, s.io)
        ComponentType.PHOTO_FIELD, ComponentType.VIDEO_FIELD -> MediaFieldLeaf(c, fieldLabel(c, s), s.draft)
        ComponentType.TEXT_FIELD, ComponentType.NUMBER_FIELD, ComponentType.TEXT_EDITOR, ComponentType.SECURE_FIELD -> {
            val field = c.field ?: return
            OutlinedTextField(
                value = s.draft[field].orEmpty(),
                onValueChange = { s.draft[field] = it },
                label = { Text(fieldLabel(c, s)) },
                placeholder = c.placeholder?.let { p -> { Text(p) } },
                singleLine = c.type != ComponentType.TEXT_EDITOR,
                visualTransformation = if (c.type == ComponentType.SECURE_FIELD) PasswordVisualTransformation() else VisualTransformation.None,
                modifier = Modifier.fillMaxWidth(),
            )
        }
        ComponentType.TOGGLE -> {
            val field = c.field ?: return
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(fieldLabel(c, s), Modifier.weight(1f))
                Switch(checked = s.draft[field] == "true", onCheckedChange = { s.draft[field] = it.toString() })
            }
        }
        ComponentType.PICKER -> {
            val field = c.field ?: return
            val options = s.entity?.fields?.firstOrNull { it.id == field }?.options.orEmpty()
            Text(fieldLabel(c, s), style = MaterialTheme.typography.labelLarge)
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                options.forEach { o -> FilterChip(selected = s.draft[field] == o, onClick = { s.draft[field] = o }, label = { Text(o) }) }
            }
        }
        ComponentType.STEPPER -> {
            val field = c.field ?: return
            val n = s.draft[field]?.toDoubleOrNull()?.toInt() ?: 0
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(fieldLabel(c, s), Modifier.weight(1f))
                TextButton(onClick = { s.draft[field] = (n - 1).toString() }) { Text("−") }
                Text(n.toString())
                TextButton(onClick = { s.draft[field] = (n + 1).toString() }) { Text("+") }
            }
        }
        else -> Text(
            if (c.type == ComponentType.UNKNOWN) "知らない部品: ${DesktopLeaves.typeName(c)}" else "デスクトップ版ではまだ使えない部品: ${DesktopLeaves.typeName(c)}",
            color = Color.Gray,
            style = MaterialTheme.typography.bodySmall,
        )
    }
}

@Composable
private fun Children(c: Component, s: ScreenScope, modifier: Modifier = Modifier) {
    Column(modifier, verticalArrangement = Arrangement.spacedBy(8.dp)) {
        c.title?.let { Text(it, style = MaterialTheme.typography.titleMedium) }
        c.children.forEach { Node(it, s) }
    }
}

@Composable
private fun Box1(modifier: Modifier, content: @Composable () -> Unit) {
    Column(modifier) { content() }
}

@Composable
private fun ListLeaf(c: Component, s: ScreenScope) {
    val entity = s.entityOf(c.entity)
    val rows = c.entity?.let { s.store.list(it) }.orEmpty()
    c.entity?.let { ListIoBar(it, s.io) }
    if (rows.isEmpty()) {
        Text(c.emptyText ?: "まだありません", color = Color.Gray)
        return
    }
    val titleField = c.item?.title ?: entity?.titleField
    rows.forEach { row ->
        val tap = c.onItemTap
        Column(
            Modifier.fillMaxWidth()
                .then(if (tap != null) Modifier.clickable { s.perform(tap, row.id) } else Modifier)
                .padding(vertical = 6.dp),
        ) {
            Text(titleField?.let { row.data[it] } ?: row.id)
            c.item?.subtitle?.let { f -> row.data[f]?.let { Text(it, style = MaterialTheme.typography.bodySmall, color = Color.Gray) } }
        }
        HorizontalDivider()
    }
}

private fun fieldLabel(c: Component, s: ScreenScope): String =
    c.label ?: s.entity?.fields?.firstOrNull { it.id == c.field }?.name ?: c.field.orEmpty()

private fun openBrowser(url: String) {
    runCatching { if (Desktop.isDesktopSupported()) Desktop.getDesktop().browse(URI(url)) }
}
