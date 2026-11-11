package jp.kz2601.metamon.renderer.components.lists

import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateMapOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.semantics.CustomAccessibilityAction
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.customActions
import androidx.compose.ui.semantics.onClick
import androidx.compose.ui.semantics.role
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ListItem
import jp.kz2601.metamon.dsl.expr.DSLClock
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.dsl.records.MatchBox
import jp.kz2601.metamon.renderer.ActionContext
import jp.kz2601.metamon.renderer.components.social.InterestButton
import jp.kz2601.metamon.renderer.components.social.MatchingHost
import jp.kz2601.metamon.renderer.components.social.interestTitle
import jp.kz2601.metamon.renderer.registry.RenderContext
import jp.kz2601.metamon.renderer.theme.RenderTheme
import jp.kz2601.metamon.renderer.theme.SymbolIcon
import jp.kz2601.metamon.renderer.theme.colors
import jp.kz2601.metamon.renderer.theme.renderCard
import kotlinx.coroutines.launch

/**
 * `list`: entity の記録の一覧(filter / sort / 検索は端末側)。iOS の `ListLeaf`。
 * - 行タップ = `onItemTap` を recordId 付きで実行(#1060 の dispatch)
 * - 長押し = 「削除」のメニュー(`swipeToDelete` で消せる記録だけ)。読み上げでは行の操作「削除」。消した後の「元に戻す」はホスト(AppRuntime)が出す
 * - `item.toggle`(#108): 行頭の丸で boolean を反転して即保存(楽観的 UI。失敗したら戻す)
 * - 行は 1 枚のカードに積み、行間は hairline。グリッドのセルが縦スクロールを持つので LazyColumn にはしない
 * - `item.interest`(#877): 仲間探しのプロフィールの行の右端に「つながりたい」([MatchingHost] を持つ runtime だけ)
 * 印(いいね)・通報は、共有まわりの能力(android-068・069)が入るまで出さない(Issue #859 の備考)。
 */
@Composable
fun ListLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) {
    val data = rememberLeafData(ctx.runtime)
    val query = c.searchField?.let { LocalScreenLocals.current?.values?.get(it) }
    val entityId = c.entity ?: ""
    val entity = data.entity(entityId)
    val rows = ListRows.rows(c, data, query)
    if (rows.isEmpty()) {
        EmptyPlaceholder("tray", c.emptyText ?: LIST_EMPTY_TEXT, modifier)
        return
    }
    val toggleField = ListToggle.field(c.item, entity)
    val overrides = remember { mutableStateMapOf<String, Boolean>() }
    val scope = rememberCoroutineScope()
    val today = DSLClock.now()
    val matching = (ctx.runtime as? MatchingHost)?.takeIf { c.item?.interest == true }
    matching?.matchingVersion?.collectAsState()?.value // 送った・つながったで描き直す
    if (matching != null) LaunchedEffect(entityId) { matching.refreshMatches(MatchBox.entries.toSet()) }
    Column(modifier.renderCard(padding = 0.dp)) {
        rows.forEachIndexed { index, row ->
            if (index > 0) HorizontalDivider(Modifier.padding(start = separatorInset(c.item)), color = RenderTheme.colors.hairline)
            val editable = ctx.runtime.canEdit(entityId, row)
            val daily = ListToggle.isDaily(c.item)
            val toggleOn = toggleField?.let { ListToggle.isOn(row, it.id, daily, overrides, today) }
            val onToggle: (() -> Unit)? = toggleField?.let { f ->
                {
                    if (editable) {
                        val next = !ListToggle.isOn(row, f.id, daily, overrides, today)
                        overrides[row.id] = next
                        scope.launch {
                            ctx.runtime.updateRecord(entityId, row.id, ListToggle.patch(row, f.id, next, daily, today))
                            // 成功ならストアの値が新しい値に、失敗なら元の値に戻る。連打で別の値になっていたら残す
                            if (overrides[row.id] == next) overrides.remove(row.id)
                        }
                    }
                }
            }
            val deletable = c.swipeToDelete == true && ctx.runtime.canDelete(entityId, row)
            val interest = matching?.interestState(entityId, row)
            val sendInterest = { scope.launch { matching?.sendInterest(entityId, row) }; Unit }
            val interestLabel = interest?.let { interestTitle(it) }
            val interestBusy = matching?.interestBusy(row) ?: false
            ListRow(
                item = c.item,
                record = row,
                entity = entity,
                data = data,
                showsChevron = c.onItemTap != null,
                toggleOn = toggleOn,
                toggleEnabled = editable,
                onToggle = onToggle,
                onTap = c.onItemTap?.let { a -> { ctx.dispatch(a, ActionContext(ctx.route, ctx.form, recordId = row.id)) } },
                onDelete = if (deletable) {
                    { ctx.dispatch(Action(type = ActionType.DELETE_RECORD, entity = entityId), ActionContext(ctx.route, ctx.form, recordId = row.id)) }
                } else {
                    null
                },
                accessory = interest?.let { st -> { InterestButton(st, interestBusy, sendInterest) } },
                accessoryActions = if (interest?.tappable == true && interestLabel != null && !interestBusy) {
                    listOf(CustomAccessibilityAction(interestLabel) { sendInterest(); true })
                } else {
                    emptyList()
                },
            )
        }
    }
}

/** 先頭のアイコン / 画像の幅ぶん区切り線を内側に寄せる。 */
private fun separatorInset(item: ListItem?): androidx.compose.ui.unit.Dp {
    val check = if (item?.toggle != null) 28.dp + RenderTheme.Spacing.row else 0.dp
    return when {
        item?.image != null -> RenderTheme.Spacing.md + check + 52.dp + RenderTheme.Spacing.row
        item?.icon != null -> RenderTheme.Spacing.md + check + 36.dp + RenderTheme.Spacing.row
        else -> RenderTheme.Spacing.md + check
    }
}

/**
 * 一覧の 1 行(title / subtitle / trailing / icon)。iOS の `ListRow`。
 * 読み上げは行 1 要素で「title, subtitle, trailing」。チェックの状態は stateDescription、チェック・削除は行の操作。
 */
@OptIn(ExperimentalFoundationApi::class)
@Composable
internal fun ListRow(
    item: ListItem?,
    record: Record,
    entity: Entity?,
    data: LeafData,
    showsChevron: Boolean = false,
    toggleOn: Boolean? = null,
    toggleEnabled: Boolean = true,
    onToggle: (() -> Unit)? = null,
    onTap: (() -> Unit)? = null,
    onDelete: (() -> Unit)? = null,
    /** 行の右端(chevron の前)。仲間探しのボタンなど(#877) */
    accessory: (@Composable () -> Unit)? = null,
    /** 右端の操作を読み上げの行の操作にも出す */
    accessoryActions: List<CustomAccessibilityAction> = emptyList(),
    /** 長押し。あれば削除のメニューの代わりに呼ぶ(呼んだ側がメニューを出す) */
    onLongPress: (() -> Unit)? = null,
) {
    val texts = ListRows.texts(item, record, entity, data)
    val checked = toggleOn == true
    var menu by remember { mutableStateOf(false) }
    val trailingNumeric = item?.trailing?.let { entity?.field(it)?.type?.isNumeric } ?: false
    val categoryColor = listOfNotNull(item?.subtitle, item?.title).firstNotNullOfOrNull { f ->
        if (entity?.field(f)?.type == FieldType.ENUM) data.text(record, f, entity).takeIf { it.isNotEmpty() } else null
    }?.let { RenderTheme.colors.category(it) }
    val label = listOfNotNull(texts.title, texts.subtitle, texts.trailing).filter { it.isNotEmpty() }.joinToString(", ")
    val actions = buildList {
        if (onToggle != null) add(CustomAccessibilityAction(if (checked) "チェックを外す" else "チェックする") { onToggle(); true })
        if (onDelete != null) add(CustomAccessibilityAction("削除") { onDelete(); true })
        addAll(accessoryActions)
    }
    Box {
        Row(
            Modifier
                .fillMaxWidth()
                .combinedClickable(
                    enabled = onTap != null || onDelete != null || onLongPress != null,
                    onClick = { onTap?.invoke() },
                    onLongClick = onLongPress ?: onDelete?.let { { menu = true } },
                )
                .clearAndSetSemantics {
                    contentDescription = label
                    toggleOn?.let { stateDescription = if (it) "チェック済み" else "未チェック" }
                    if (onTap != null) {
                        role = Role.Button
                        onClick("開く") { onTap(); true }
                    }
                    if (actions.isNotEmpty()) customActions = actions
                }
                .padding(horizontal = RenderTheme.Spacing.md, vertical = RenderTheme.Spacing.row),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.row),
        ) {
            if (toggleOn != null) CheckCircle(toggleOn, toggleEnabled, onToggle)
            when {
                item?.icon != null -> Box(
                    Modifier.size(36.dp).background(RenderTheme.colors.brandSoft, RoundedCornerShape(RenderTheme.Radius.inner)),
                    contentAlignment = Alignment.Center,
                ) { SymbolIcon(item.icon, 20.dp, RenderTheme.colors.brandA) }
                categoryColor != null -> Box(
                    Modifier.size(10.dp).background(categoryColor, CircleShape).border(1.dp, androidx.compose.ui.graphics.Color.White.copy(alpha = 0.6f), CircleShape),
                )
            }
            Column(Modifier.weight(1f).alpha(if (checked) 0.6f else 1f), verticalArrangement = Arrangement.spacedBy(3.dp)) {
                Text(
                    texts.title,
                    style = RenderTheme.Fonts.rowTitle.copy(textDecoration = if (checked) TextDecoration.LineThrough else null),
                    color = if (checked) MaterialTheme.colorScheme.onSurfaceVariant else MaterialTheme.colorScheme.onSurface,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
                texts.subtitle?.let {
                    Text(it, style = RenderTheme.Fonts.rowSubtitle, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis)
                }
            }
            texts.trailing?.let {
                Text(
                    it,
                    style = if (trailingNumeric) RenderTheme.Fonts.rowTrailing else MaterialTheme.typography.bodyMedium,
                    color = if (trailingNumeric) MaterialTheme.colorScheme.onSurface else MaterialTheme.colorScheme.onSurfaceVariant,
                    maxLines = 1,
                )
            }
            accessory?.invoke()
            if (showsChevron) SymbolIcon("chevron.right", 16.dp, RenderTheme.colors.tertiaryLabel)
        }
        DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
            DropdownMenuItem(
                text = { Text("削除", color = RenderTheme.colors.negative) },
                leadingIcon = { SymbolIcon("trash", 20.dp, RenderTheme.colors.negative) },
                onClick = {
                    menu = false
                    onDelete?.invoke()
                },
            )
        }
    }
}

/** 行頭のチェック丸。行タップとは別のボタン(48dp の当たり判定)。読み上げは行の操作で行う。 */
@Composable
private fun CheckCircle(on: Boolean, enabled: Boolean, onToggle: (() -> Unit)?) {
    IconButton(onClick = { onToggle?.invoke() }, modifier = Modifier.size(28.dp + 16.dp).padding(0.dp)) {
        if (on) {
            Icon(Icons.Filled.CheckCircle, contentDescription = null, tint = RenderTheme.colors.brandA, modifier = Modifier.size(28.dp).alpha(if (enabled) 1f else 0.4f))
        } else {
            Box(Modifier.size(26.dp).alpha(if (enabled) 1f else 0.4f).border(2.dp, RenderTheme.colors.secondaryLabel, CircleShape))
        }
    }
}

/** 空の一覧・表・時系列・グラフの案内(iOS の `EmptyPlaceholder` の「空」の形)。読み込み中・取得失敗の形はホストの読み込み状態が入ってから。 */
@Composable
internal fun EmptyPlaceholder(icon: String, text: String, modifier: Modifier = Modifier) {
    Column(
        modifier.fillMaxWidth().padding(vertical = RenderTheme.Spacing.lg, horizontal = RenderTheme.Spacing.md),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm),
    ) {
        SymbolIcon(icon, 32.dp, RenderTheme.colors.tertiaryLabel)
        Text(
            text,
            style = MaterialTheme.typography.bodyMedium.copy(fontWeight = FontWeight.Medium),
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            textAlign = androidx.compose.ui.text.style.TextAlign.Center,
        )
    }
}
