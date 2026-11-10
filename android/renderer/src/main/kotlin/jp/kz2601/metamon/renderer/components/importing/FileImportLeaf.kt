package jp.kz2601.metamon.renderer.components.importing

import android.content.Context
import android.net.Uri
import android.provider.OpenableColumns
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.renderer.R
import jp.kz2601.metamon.renderer.components.actions.ActionButton
import jp.kz2601.metamon.renderer.components.actions.ActionButtonKind
import jp.kz2601.metamon.renderer.components.actions.ActionNotice
import jp.kz2601.metamon.renderer.form.FormChoices
import jp.kz2601.metamon.renderer.registry.RenderContext
import jp.kz2601.metamon.renderer.theme.RenderTheme
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/**
 * `fileImport`(#865・iOS の FileImportLeaf): ボタンでファイルを選び(Storage Access Framework・権限の確認なし)、entity に記録を足す。
 * - csv: 1 行 = 1 件(列の当て方・型の変換は :dsl の RecordImport)。足す前に「n 件を追加・m 行は読めない」を確かめる。
 * - text: 本文を `field` に入れた記録を 1 件。
 * - pdf / audio: この端末ではまだ読めない(選べる種類から外し、ボタンの下に一言)。
 * ファイルは読むだけで、送らない・コピーしない・残さない。足すのは [RecordImportHost.addRecord](フォームの保存と同じく端末に入り同期)。
 */
@Composable
fun FileImportLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val runtime = ctx.runtime
    val host = runtime as? RecordImportHost
    val entity = c.entity?.let { runtime.entityModel(it) }
    var reading by remember { mutableStateOf(false) }
    var progress by remember { mutableStateOf<Pair<Int, Int>?>(null) }
    var pending by remember { mutableStateOf<ImportStep.Confirm?>(null) }
    var problem by remember { mutableStateOf<ImportProblem?>(null) }
    var status by remember { mutableStateOf<String?>(null) }

    if (entity == null || entity.scope == "public" || entity.single) {
        ActionNotice(stringResource(R.string.renderer_import_unusable), modifier)
        return
    }
    val mimes = FileImportFlow.mimeTypes(c.accept)
    val usable = host != null && FileImportFlow.usable(c, entity)

    val picker = rememberLauncherForActivityResult(ActivityResultContracts.OpenDocument()) { uri ->
        if (uri == null || host == null) return@rememberLauncherForActivityResult
        scope.launch {
            reading = true
            try {
                val refs = entity.fields.mapNotNull { it.refEntity }.toSet()
                host.loadRecords(refs + entity.id)
                val step = read(context, uri, c, entity, ctx)
                when (step) {
                    is ImportStep.Confirm -> pending = step
                    is ImportStep.Problem -> problem = step.problem
                }
            } finally {
                reading = false
            }
        }
    }

    val p = progress
    val label = when {
        p != null -> stringResource(R.string.renderer_import_adding, p.first, p.second)
        reading -> stringResource(R.string.renderer_import_reading)
        else -> c.label?.takeIf { it.isNotEmpty() } ?: stringResource(R.string.renderer_import_button)
    }
    Column(modifier, verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.xs)) {
        ActionButton(
            label = label,
            kind = ActionButtonKind.SECONDARY,
            onClick = {
                status = null
                picker.launch(mimes.toTypedArray())
            },
            modifier = Modifier.testTag("fileImport.button"),
            icon = "doc",
            enabled = usable && !reading && p == null,
            running = reading || p != null,
        )
        if (c.accept.any { it in FileImportFlow.unavailableKinds }) {
            Text(stringResource(R.string.renderer_import_kind_note), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
        status?.let {
            Text(it, Modifier.testTag("fileImport.status"), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
    }

    pending?.let { step ->
        val res = context.resources
        AlertDialog(
            onDismissRequest = { pending = null },
            title = { Text(stringResource(R.string.renderer_import_confirm_title)) },
            text = { Text(FileImportCopy.message(res, step.message)) },
            confirmButton = {
                TextButton(onClick = {
                    pending = null
                    if (host == null) return@TextButton
                    scope.launch {
                        val total = step.rows.size
                        progress = 0 to total
                        val added = FileImportFlow.add(host, entity.id, step.rows) { progress = it to total }
                        progress = null
                        status = FileImportCopy.result(res, added, total)
                    }
                }) { Text(stringResource(R.string.renderer_import_confirm_add)) }
            },
            dismissButton = { TextButton(onClick = { pending = null }) { Text(stringResource(R.string.renderer_import_cancel)) } },
        )
    }
    problem?.let { pr ->
        AlertDialog(
            onDismissRequest = { problem = null },
            title = { Text(stringResource(R.string.renderer_import_problem_title)) },
            text = { Text(FileImportCopy.problem(context.resources, pr)) },
            confirmButton = { TextButton(onClick = { problem = null }) { Text(stringResource(R.string.renderer_import_ok)) } },
        )
    }
}

/** 選んだファイルを読み、確認する中身にする。上限 + 1 バイトまでしか読まない。 */
private suspend fun read(context: Context, uri: Uri, c: Component, entity: Entity, ctx: RenderContext): ImportStep =
    withContext(Dispatchers.IO) {
        val resolver = context.contentResolver
        val name = runCatching {
            resolver.query(uri, arrayOf(OpenableColumns.DISPLAY_NAME), null, null, null)?.use { cur ->
                if (cur.moveToFirst()) cur.getString(0) else null
            }
        }.getOrNull() ?: uri.lastPathSegment.orEmpty()
        val kind = FileImportFlow.kind(name, resolver.getType(uri), c.accept)
            ?: return@withContext ImportStep.Problem(ImportProblem.Unsupported)
        if (kind in setOf(ImportKind.PDF, ImportKind.AUDIO)) return@withContext ImportStep.Problem(ImportProblem.KindUnavailable)
        val limit = FileImportFlow.byteLimit(kind)
        val bytes = runCatching {
            resolver.openInputStream(uri)?.use { input -> input.readNBytes(limit + 1) }
        }.getOrNull() ?: return@withContext ImportStep.Problem(ImportProblem.CannotOpen)
        val existing = FormChoices.records(ctx.runtime, entity.id).size
        FileImportFlow.prepare(bytes, kind, name, c, entity, existing) { field, ref -> FileImportFlow.refId(ctx.runtime, field, ref) }
    }
