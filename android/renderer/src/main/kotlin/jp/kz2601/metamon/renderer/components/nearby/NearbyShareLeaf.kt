package jp.kz2601.metamon.renderer.components.nearby

import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.semantics.testTag
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentMode
import jp.kz2601.metamon.dsl.nearby.NearbyShare
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.renderer.R
import jp.kz2601.metamon.renderer.components.UnavailableComponent
import jp.kz2601.metamon.renderer.components.actions.ActionButton
import jp.kz2601.metamon.renderer.components.actions.ActionButtonKind
import jp.kz2601.metamon.renderer.form.FormDraft
import jp.kz2601.metamon.renderer.registry.RenderContext

/**
 * `nearbyShare`(#36・iOS の `NearbyShareLeaf`): 近くの Tailor It 端末へ記録を送るボタン。通信と相手選びはホスト([NearbyHost])。
 * 送る範囲: フォーム画面(screen.entity == entity)なら入力中の 1 件、それ以外はその entity の新しい順 最大 50 件。
 * 押した時に近くのデバイスの権限を聞く(断られたら開かない)。mode:passby(すれ違い通信)はまだ Android に無い(#1383)。
 */
@Composable
fun NearbyShareLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) {
    if (c.mode == ComponentMode.PASSBY) {
        UnavailableComponent(c, modifier)
        return
    }
    val rt = ctx.runtime
    val entity = c.entity.orEmpty()
    val form = ctx.form?.takeIf { it.entity.id == entity }
    if (form == null) LaunchedEffect(entity) { if (entity.isNotEmpty()) rt.loadRecords(setOf(entity)) }
    val all by (rt.recordsFlow?.collectAsState() ?: remember { mutableStateOf(emptyMap<String, List<Record>>()) })
    val rows = NearbySendRange.records(form, all[entity].orEmpty())
    var pending by remember { mutableStateOf<List<Map<String, JSONValue>>?>(null) }
    val ask = LocalNearbyPermission.current { granted ->
        val send = pending
        pending = null
        if (granted && send != null) rt.nearby?.shareNearby(entity, send)
    }
    val hint = if (rows.isEmpty()) stringResource(R.string.nearby_nothing_to_send)
    else pluralStringResource(R.plurals.nearby_send_hint, rows.size, rows.size)
    ActionButton(
        label = c.label ?: stringResource(R.string.nearby_send_default),
        kind = ActionButtonKind.SECONDARY,
        icon = "iphone",
        enabled = rows.isNotEmpty(),
        onClick = {
            pending = rows
            ask()
        },
        modifier = modifier.semantics {
            testTag = "nearby.send.${c.id ?: entity}"
            stateDescription = hint
        },
    )
}

/** 送る範囲(iOS の `NearbyShareLeaf.records`)。View から切り離してテストする。 */
object NearbySendRange {
    fun records(form: FormDraft?, records: List<Record>): List<Map<String, JSONValue>> {
        if (form != null) return form.payload.takeIf { it.isNotEmpty() }?.let { listOf(it) } ?: emptyList()
        return NearbyShare.recordsToSend(records)
    }
}
