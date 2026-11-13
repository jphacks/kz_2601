package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.desktop.store.KeyValueRetentionConsent
import jp.kz2601.metamon.desktop.store.RetentionRunner
import jp.kz2601.metamon.desktop.store.SessionPersistence
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateMapOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.Screen
import jp.kz2601.metamon.dsl.logic.CsvReader
import jp.kz2601.metamon.dsl.logic.ImportRowFailure
import jp.kz2601.metamon.desktop.io.RecordIo
import jp.kz2601.metamon.dsl.records.Aggregator
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.dsl.records.RecordPermission
import jp.kz2601.metamon.dsl.records.RecordQuery
import jp.kz2601.metamon.dsl.records.SingleRecord
import jp.kz2601.metamon.dsl.records.entityModels
import jp.kz2601.metamon.dsl.screen
import jp.kz2601.metamon.renderer.form.FormDraft
import jp.kz2601.metamon.renderer.form.FormPrefill
import jp.kz2601.metamon.renderer.navigation.NavigationResolver
import jp.kz2601.metamon.renderer.navigation.NavigationState
import jp.kz2601.metamon.renderer.navigation.Presentation
import jp.kz2601.metamon.renderer.navigation.Route
import jp.kz2601.metamon.renderer.navigation.top
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.doubleOrNull

/** アクションを押した場所(画面・下書き・押した行の recordId)。 */
data class ActionContext(val route: Route, val form: FormDraft?, val recordId: String? = null) {
    /** 親から子を作る時の親(押した行 → 編集中の記録)。 */
    val currentRecordId: String? get() = recordId ?: form?.recordId
}

/** OS の機能に頼むアクション(URL・カレンダー・本・通知・書き出し・音声・ライブアクティビティ)の受け口。返り値は画面に出す一言。 */
fun interface ActionHost {
    fun request(action: Action, ctx: ActionContext, session: DesktopSession): String?
}

/**
 * デスクトップ版の画面の状態と操作(iOS の AppRuntime・Android の ConformanceTest の CaseSession と同じ規則)。
 * 遷移は Android と同じ [NavigationResolver]、下書きは [FormDraft](どちらも android/renderer のソースをそのまま取り込む)。
 * 画面(DesktopApp)と適合テストが同じこのクラスで操作するので、steps の比べ方がそのまま画面の動きになる。
 */
class DesktopSession(
    val dsl: AppDSL,
    initialStore: MockStore = MockStore(),
    val uid: String? = "me",
    startScreen: String? = null,
    var host: ActionHost? = null,
    /** 端末に残す・送る(#1887)。null はメモリだけ(適合テスト・開発用の記録の JSON)。[shared] を渡した時はそちらの物を使う。 */
    persistence: SessionPersistence? = null,
    /** 同じアプリの他のウィンドウと共有する記録(#1763)。null ならこのセッションだけの物を作る。 */
    shared: SharedRecords? = null,
    /** 記録の詳細のウィンドウ(#1763): [startScreen] をこの記録で開き、そこから戻るとウィンドウを閉じる。 */
    startRecordId: String? = null,
    /** 記録の保存期間(#482)。既定は端末の設定に確認を覚える(端末に残さない時は使わない)。 */
    retention: RetentionRunner? = null,
) {
    val models: List<Entity> = dsl.entityModels()

    /** 記録(同じアプリのウィンドウで共有)。 */
    val records: SharedRecords = shared ?: SharedRecords(initialStore, persistence)
    val persistence: SessionPersistence? get() = records.persistence

    val retention: RetentionRunner? = retention ?: records.persistence?.let { RetentionRunner(KeyValueRetentionConsent(it.store.settings)) }

    /** 保存した回数(#482)。DesktopApp が見て、保存の後に [purgeExpired] を呼ぶ。 */
    var saves: Int by mutableIntStateOf(0)
        private set

    /**
     * 保存期間を過ぎた記録を消す(#482): 端末の記録を読み終えた時(DSL を当てた時)・ウィンドウが前に来た時・保存の後に DesktopApp が呼ぶ。
     * 確認済みの entity だけ。画面の記録から消すので、通常の削除と同じく端末と送信待ちに積む。端末に残さない時・読み込み中・消している最中は何もしない。消した件数。
     */
    suspend fun purgeExpired(): Int {
        val runner = retention ?: return 0
        val p = persistence ?: return 0
        if (records.loading || records.wiping) return 0
        val gone = runner.expired(p.appId, models, p.retentionRows(), uid)
        if (gone.isEmpty() || records.loading || records.wiping) return 0
        var next = store
        var count = 0
        for ((entity, ids) in gone) {
            for (id in ids) {
                if (next.find(entity, id) == null) continue
                next = next.delete(entity, id)
                count++
            }
        }
        if (count > 0) store = next
        return count
    }

    /** 画面が見る記録。書き換えると前後の差を [persistence] に渡す。 */
    var store: MockStore
        get() = records.store
        private set(value) = records.update(models, value)
    var nav: NavigationState by mutableStateOf(NavigationState.initial(dsl))
        private set

    /** 詳細のウィンドウの根(ここから戻るとウィンドウを閉じる)。アプリのウィンドウは null。 */
    var detailRoot: Route? = null
        private set

    /** 詳細のウィンドウの根から戻った(ウィンドウを閉じる)。 */
    var onCloseWindow: (() -> Unit)? = null

    /** 遷移はここを通す(閉じた画面の下書きを捨てる)。 */
    private fun go(next: NavigationState) {
        nav = next
        val live = liveRouteIds()
        drafts.keys.retainAll(live)
        gone.retainAll(live)
    }

    /** 画面に出す一言(保存できない理由・OS の機能の結果)。 */
    var message: String? by mutableStateOf(null)

    /** 起きたこと(適合テストの effects と同じ形)。 */
    val effects = mutableListOf<JsonElement>()

    /** 下書きと、作った時の記録(変わったら作り直す)。 */
    private class DraftEntry(val draft: FormDraft, val source: Row?)

    private val drafts = HashMap<String, DraftEntry>()

    /** 元の記録が他のウィンドウで消えた route の id(下書きは捨て、保存させない)。 */
    private val gone = HashSet<String>()
    private val queries = HashMap<String, MutableMap<String, String>>()

    init {
        if (startScreen != null && dsl.screen(startScreen) != null && (startScreen != top.screen || startRecordId != null)) {
            go(NavigationResolver.navigate(nav, startScreen, Presentation.PUSH, startRecordId, top, dsl))
            if (startRecordId != null && top.recordId == startRecordId) detailRoot = top
        }
    }

    val top: Route get() = nav.top(dsl)

    /** 取り込み・貼り付け(io/DesktopIo)が記録を差し替える。 */
    fun replaceStore(next: MockStore) {
        store = next
    }

    /** サインアウト・端末のデータを消す(#1887): 端末の記録・送り待ち・設定を消し、画面を空にして最初の画面へ。 */
    suspend fun wipeLocalData() {
        records.wipe()
        followWipe()
    }

    private var seenWipes = records.wipes

    /**
     * 同じ記録を見るどこか(他のウィンドウ・パレットの「この端末のデータを消す」)で消した後: 下書きを捨てて最初の画面へ。
     * 詳細のウィンドウは記録が無くなったので閉じる。
     */
    fun followWipe() {
        if (records.wipes == seenWipes) return
        seenWipes = records.wipes
        drafts.clear()
        go(NavigationState.initial(dsl))
        if (detailRoot != null) {
            detailRoot = null
            onCloseWindow?.invoke()
        }
    }

    /**
     * 記録が変わった後(DesktopApp が記録の変わるたびに呼ぶ): 開いている記録が他のウィンドウで消えていたら、
     * 詳細のウィンドウは閉じ、アプリのウィンドウは一番上の画面を閉じて知らせる。
     */
    fun followRemoval() {
        detailRoot?.let { root ->
            form(root)
            if (root.id in gone) {
                detailRoot = null
                onCloseWindow?.invoke()
                return
            }
        }
        val t = top
        form(t)
        if (t.id in gone && NavigationResolver.canClose(nav, t)) {
            go(NavigationResolver.close(nav, t))
            message = Strings["desktop_record_deleted_elsewhere"]
        }
    }

    fun entityModel(id: String?): Entity? = id?.let { i -> models.firstOrNull { it.id == i } }

    fun records(entity: String): List<Record> = store.records(entity, entityModel(entity))

    /** 画面は [DataLabels.current] を渡す(既定は描画の木と同じ日本語)。 */
    fun leafData(labels: DataLabels = DataLabels.JAPANESE): LeafData = LeafData(models, ::records, uid, labels)

    /**
     * route の下書き(entity の無い画面は null)。開いた時に 1 回作り、保存するか閉じるまで入力を残す。
     * 元の記録が変わったら作り直す(打ちかけの入力がある時は残す)。新規は開いた時に FormPrefill を受け取る。
     */
    fun form(route: Route): FormDraft? {
        val entity = entityModel(dsl.screen(route.screen)?.entity) ?: return null
        val source = sourceRow(entity, route)
        drafts[route.id]?.let { e ->
            if (e.source != null && source == null && !entity.single) {
                // 元の記録が他のウィンドウで消えた: 下書きを捨て、保存できない形にする(保存すると別の新しい記録になるため)
                drafts.remove(route.id)
                gone += route.id
            } else if (e.source == source || source == null || e.draft.hasUnsavedChanges) {
                return e.draft
            }
        }
        if (route.id in gone && source == null) return FormDraft(entity, null).also { it.isReadOnly = true }
        gone -= route.id
        val record = source?.let { row -> records(entity.id).firstOrNull { it.id == row.id } }
        val draft = FormDraft(entity, record)
        if (record != null) {
            draft.isReadOnly = !RecordPermission.canEdit(record, entity, uid)
            draft.canDelete = !entity.single && RecordPermission.canDelete(record, entity, uid)
        } else {
            draft.prefill(FormPrefill.consume(entity))
        }
        drafts[route.id] = DraftEntry(draft, source)
        return draft
    }

    /** 下書きの元の記録(single は 1 件・他は route の recordId)。 */
    private fun sourceRow(entity: Entity, route: Route): Row? {
        val id = if (entity.single) SingleRecord.pick(records(entity.id))?.id else route.recordId
        return id?.let { store.find(entity.id, it) }
    }

    /** 今どこかのスタック・sheet・fullScreen・タブの根にある route の id(下書きを残す物)。 */
    internal fun liveRouteIds(): Set<String> = buildSet {
        nav.paths.values.forEach { routes -> routes.forEach { add(it.id) } }
        nav.sheet?.let { add(it.id) }
        nav.fullScreen?.let { add(it.id) }
        dsl.app.tabs.forEach { add(Route(it.screen, stack = Route.tabStack(it.screen)).id) }
        add(Route(dsl.initialScreen).id)
        add(top.id)
    }

    /** 下書きを持っている route の id(テスト用)。 */
    internal val draftRouteIds: Set<String> get() = drafts.keys.toSet()

    /**
     * 一覧のチェックなど、1 件の記録の field を書き換える。フォームと同じ記録の規則(作った人だけ・membersCanEdit)を通す。
     * 書けたら true。書けない時は理由を [message] に出す。
     */
    fun updateRecord(entity: String, id: String, patch: Map<String, String>): Boolean {
        val row = store.find(entity, id) ?: return false
        if (!canEditRecord(entity, id)) {
            message = Strings["runtime_read_only"]
            return false
        }
        store = store.save(entity, id, row.data + patch)
        return true
    }

    /** 1 件書き換えられるか(フォームの読み取りだけと同じ規則)。 */
    fun canEditRecord(entity: String, id: String): Boolean {
        val record = records(entity).firstOrNull { it.id == id } ?: return false
        return RecordPermission.canEdit(record, entityModel(entity), uid)
    }

    /** 1 件消せるか(フォームの削除ボタンと同じ規則・single は消さない)。 */
    fun canDeleteRecord(entity: String, id: String): Boolean {
        val model = entityModel(entity)
        if (model?.single == true) return false
        val record = records(entity).firstOrNull { it.id == id } ?: return false
        return RecordPermission.canDelete(record, model, uid)
    }

    /** 検索欄の文字(route ごと)。 */
    fun queries(route: Route): MutableMap<String, String> = queries.getOrPut(route.id) { mutableStateMapOf() }

    /** `list` の行(filter → sort → 検索)。iOS の `ListLeaf.rows`。 */
    fun rows(c: Component, query: String?): List<Record> = ListRows.rows(c, leafData(), query)

    /** 部品の state(押せるか・描くか)。[DesktopLeaves.state] に今の人を渡す。 */
    fun state(c: Component, form: FormDraft?): String = DesktopLeaves.state(c, form, uid)

    // MARK: - 操作

    /** 部品・toolbar・行を押した。確認(confirmMessage)は呼ぶ側が「はい」を受けてから呼ぶ。 */
    fun dispatch(action: Action, ctx: ActionContext) {
        message = null
        action.confirmMessage?.let { effects += obj("confirm" to JsonPrimitive(it)) }
        perform(action, ctx)
    }

    /**
     * openUrlButton: 候補を先頭から openUrl(link と同じ・許可した scheme だけ)で開き、開けたら止める。全部だめなら最後の理由が残る。
     * effects は 1 回押して 1 つ(開けた候補、全部だめなら最後に試した候補)。開けなかった候補の分は消す。
     */
    fun openFirst(urls: List<String>, ctx: ActionContext) {
        for ((i, url) in urls.withIndex()) {
            val mark = effects.size
            dispatch(Action(type = ActionType.OPEN_URL, url = url), ctx)
            if (message == null) return
            if (i < urls.lastIndex) while (effects.size > mark) effects.removeAt(effects.lastIndex)
        }
    }

    fun back(route: Route = top) {
        if (route == detailRoot) {
            onCloseWindow?.invoke()
            return
        }
        go(NavigationResolver.close(nav, route))
    }

    /** 戻る・閉じるを出すか(詳細のウィンドウの根は「閉じる」)。 */
    fun canClose(route: Route): Boolean = route == detailRoot || NavigationResolver.canClose(nav, route)

    /** タブを選ぶ。 */
    fun selectTab(screen: String) {
        go(nav.copy(selectedTab = screen))
    }

    /** 移った先が新規フォームなら、その場で FormPrefill を受け取る。受け取る新規が無ければ捨てる(別の画面に漏らさない)。 */
    private fun navigate(to: String, action: Action, recordId: String?, from: Route) {
        go(NavigationResolver.navigate(nav, to, Presentation.of(action.presentation?.wire), recordId, from, dsl))
        form(top)
        models.firstOrNull()?.let(FormPrefill::consume)
    }

    private fun perform(action: Action, ctx: ActionContext) {
        when (action.type) {
            ActionType.NAVIGATE -> {
                val to = action.screen ?: return
                val ref = action.prefillRef
                if (ref == null) {
                    navigate(to, action, ctx.recordId, ctx.route)
                    return
                }
                val parentId = ctx.currentRecordId
                if (parentId == null) {
                    effects += obj("rejected" to JsonPrimitive("unsavedParent"))
                    message = Strings["runtime_save_parent_first"]
                    return
                }
                FormPrefill.set(mapOf(ref to JSONValue.Str(parentId)))
                navigate(to, action, null, ctx.route)
            }
            ActionType.BACK -> back(ctx.route)
            ActionType.SAVE_RECORD -> save(action, ctx)
            ActionType.DELETE_RECORD -> {
                val entity = action.entity ?: ctx.form?.entity?.id ?: return
                val id = ctx.recordId ?: ctx.form?.recordId ?: return
                // 一覧の行の削除もフォームの削除ボタンと同じ記録の規則を通す
                if (store.find(entity, id) != null && !canDeleteRecord(entity, id)) {
                    effects += obj("rejected" to JsonPrimitive("notAllowed"))
                    message = Strings["desktop_delete_not_allowed"]
                    return
                }
                store = store.delete(entity, id)
                effects += obj("deleted" to obj("entity" to JsonPrimitive(entity), "recordId" to JsonPrimitive(id)))
                if (ctx.recordId == null) {
                    drafts.entries.removeAll { it.value.draft === ctx.form }
                    back(ctx.route)
                }
            }
            else -> {
                // OS の機能を呼ぶもの(URL・通知・書き出し・ライブアクティビティ・音声)は「何を頼んだか」を残し、ホストに渡す
                effects += obj("request" to actionJson(action))
                if (action.type == ActionType.RESET_FORM) ctx.form?.reset()
                message = host?.request(action, ctx, this) ?: message
            }
        }
    }

    private fun save(action: Action, ctx: ActionContext) {
        val form = ctx.form
        val entity = action.entity ?: form?.entity?.id
        if (form == null || entity == null) {
            effects += obj("rejected" to JsonPrimitive("noForm"))
            message = Strings["runtime_no_form"]
            return
        }
        // 開いている間に他のウィンドウで消された: 保存すると別の新しい記録になって生き返るので断る
        val formId = form.recordId
        if ((formId != null && store.find(entity, formId) == null) || ctx.route.id in gone) {
            effects += obj("rejected" to JsonPrimitive("deleted"))
            message = Strings["desktop_record_deleted_elsewhere"]
            return
        }
        if (form.isReadOnly) {
            message = Strings["runtime_read_only"]
            return
        }
        val missing = form.flagMissingRequired()
        if (missing.isNotEmpty()) {
            effects += obj("rejected" to obj("missing" to JsonArray(missing.map(::JsonPrimitive))))
            message = Strings.format("runtime_missing_required", missing.joinToString(Strings["desktop_list_separator"]))
            return
        }
        val payload = form.payload
        val existing = formId
        // 同じ記録を別のウィンドウが先に保存していた: 後から保存した方が勝ち、上書きしたことを知らせる(#1763)
        val overwrote = existing != null && drafts.values.firstOrNull { it.draft === form }?.source?.let { it != store.find(entity, existing) } == true
        // 端末に残す時は送れた後もたどれる仮 id(local-…)。メモリだけの時は適合テストと同じ new-N
        val id = existing ?: persistence?.newRecordId() ?: "new-${store.nextId}"
        store = store.save(entity, existing, payload.mapValues { MockStore.cell(it.value) }, newId = id)
        effects += obj(
            "saved" to obj(
                "entity" to JsonPrimitive(entity),
                "recordId" to JsonPrimitive(id),
                "data" to JsonObject(payload.mapValues { it.value.toJsonElement() }),
            ),
        )
        form.markSaved(queuedOffline = false)
        drafts.entries.removeAll { it.value.draft === form } // 保存した画面の下書きは捨てる(開き直すと新規)
        val to = action.screen
        if (to != null) navigate(to, action, null, ctx.route) else back(ctx.route)
        if (overwrote) message = Strings["desktop_window_conflict"]
        saves++
    }

    /** 適合テストの操作 1 つ(case.json の operations)。対象が無い・見えていないなら false。 */
    fun apply(op: JsonObject): Boolean {
        val route = top
        val form = form(route)
        val screen = dsl.screen(route.screen)
        val ctx = ActionContext(route, form)
        when {
            op.containsKey("tap") -> {
                val id = (op["tap"] as? JsonPrimitive)?.contentOrNull
                val c = screen?.allComponents()?.firstOrNull { it.id == id } ?: return false
                val a = c.action ?: return false
                if (state(c, form) != "rendered") return false
                dispatch(a, ctx)
            }
            op.containsKey("tapToolbar") -> {
                val label = (op["tapToolbar"] as? JsonPrimitive)?.contentOrNull
                val tb = screen?.toolbar?.firstOrNull { it.label == label } ?: return false
                dispatch(tb.action, ctx)
            }
            op.containsKey("tapRow") -> {
                val row = op["tapRow"] as? JsonObject ?: return false
                val id = (row["component"] as? JsonPrimitive)?.contentOrNull
                val c = screen?.allComponents()?.firstOrNull { if (id == null) it.onItemTap != null else it.id == id } ?: return false
                val a = c.onItemTap ?: return false
                if (state(c, form) != "rendered") return false
                val rows = rows(c, c.searchField?.let { queries(route)[it] })
                val i = (row["index"] as? JsonPrimitive)?.doubleOrNull?.toInt() ?: 0
                val r = rows.getOrNull(i) ?: return false
                dispatch(a, ActionContext(route, form, r.id))
            }
            op.containsKey("input") -> {
                val input = op["input"] as? JsonObject ?: return false
                val f = (input["field"] as? JsonPrimitive)?.contentOrNull
                if (form == null || f == null) return false
                form.values[f] = JSONValue.from(input["value"] ?: JsonNull)
            }
            op.containsKey("search") -> {
                val search = op["search"] as? JsonObject ?: return false
                val key = (search["field"] as? JsonPrimitive)?.contentOrNull ?: return false
                queries(route)[key] = (search["text"] as? JsonPrimitive)?.contentOrNull.orEmpty()
            }
            op.containsKey("importCsv") -> {
                // ファイル選択と確認ダイアログは含めない: CSV の中身を直に渡し、列の当て方と足す記録だけを比べる(Android の適合テストと同じ)
                val imp = op["importCsv"] as? JsonObject ?: return false
                val id = (imp["component"] as? JsonPrimitive)?.contentOrNull
                val c = screen?.allComponents()?.firstOrNull { if (id == null) it.type.wire == "fileImport" else it.id == id } ?: return false
                if (c.type.wire != "fileImport") return false
                val entity = entityModel(c.entity) ?: return false
                if (state(c, form) != "rendered") return false
                importCsv(csvText(imp["text"]), c, entity)
            }
            op.containsKey("back") -> back(route)
            else -> return false
        }
        return true
    }

    /** `importCsv.text`: 1 つの文字列、または 1 行 1 要素の配列(読みやすさのため)。 */
    private fun csvText(v: JsonElement?): String = when (v) {
        is JsonArray -> v.mapNotNull { (it as? JsonPrimitive)?.contentOrNull }.joinToString("\n") + "\n"
        is JsonPrimitive -> v.contentOrNull.orEmpty()
        else -> ""
    }

    /** fileImport の CSV の取り込み(画面の [jp.kz2601.metamon.desktop.io.DesktopIo] の計画 → 確認で足す、を確認なしで)。 */
    private fun importCsv(text: String, c: Component, entity: Entity) {
        val plan = RecordIo.plan(CsvReader.parse(text), entity, c.map.orEmpty(), store, models)
        effects += obj(
            "imported" to obj(
                "entity" to JsonPrimitive(entity.id),
                "matched" to JsonArray(plan.matched.map { obj("column" to JsonPrimitive(it.first), "field" to JsonPrimitive(it.second)) }),
                "unused" to JsonArray(plan.unused.map(::JsonPrimitive)),
                "failures" to JsonArray(
                    plan.failures.map {
                        obj(
                            "line" to JsonPrimitive(it.line),
                            "field" to JsonPrimitive(it.fieldName),
                            "kind" to JsonPrimitive(importKind(it.kind)),
                            "cell" to JsonPrimitive(it.cell),
                        )
                    },
                ),
                "overLimit" to JsonPrimitive(plan.overLimit),
            ),
        )
        for (data in plan.rows) {
            val id = persistence?.newRecordId() ?: "new-${store.nextId}"
            store = store.save(entity.id, null, data.mapNotNull { (k, v) -> RecordIo.toStore(v)?.let { k to it } }.toMap(), newId = id)
            effects += obj(
                "saved" to obj(
                    "entity" to JsonPrimitive(entity.id),
                    "recordId" to JsonPrimitive(id),
                    "data" to JsonObject(data.mapValues { it.value.toJsonElement() }),
                ),
            )
        }
    }

    private fun importKind(kind: ImportRowFailure.Kind): String = when (kind) {
        ImportRowFailure.Kind.INVALID -> "invalid"
        ImportRowFailure.Kind.REQUIRED -> "required"
        ImportRowFailure.Kind.TOO_LONG -> "tooLong"
    }

    companion object {
        /** effects の `request` に入れるアクション(Android の RenderTree.action と同じキー)。 */
        fun actionJson(a: Action): JsonObject = JsonObject(
            buildMap {
                put("type", JsonPrimitive(a.type.wire.ifEmpty { a.rawType }))
                a.screen?.let { put("screen", JsonPrimitive(it)) }
                a.presentation?.let { put("presentation", JsonPrimitive(it.wire)) }
                a.prefillRef?.let { put("prefillRef", JsonPrimitive(it)) }
                a.entity?.let { put("entity", JsonPrimitive(it)) }
                a.url?.let { put("url", JsonPrimitive(it)) }
                a.confirmMessage?.let { put("confirmMessage", JsonPrimitive(it)) }
                a.format?.let { put("format", JsonPrimitive(it.wire)) }
                a.notificationId?.let { put("notificationId", JsonPrimitive(it)) }
            },
        )

        private fun obj(vararg pairs: Pair<String, JsonElement>) = JsonObject(mapOf(*pairs))
    }
}

/**
 * 下書きを文字の map として見せる(io/DesktopIo の写真・動画・ドロップが field に file URI を書く口)。
 * 書くと文字の値・消すと空になる。
 */
class FormDraftStrings(private val form: FormDraft) : AbstractMutableMap<String, String>() {
    override fun get(key: String): String? = form.values[key]?.stringValue

    override fun put(key: String, value: String): String? = get(key).also { form.values[key] = JSONValue.Str(value) }

    override fun remove(key: String): String? = get(key).also { form.values[key] = JSONValue.Null }

    override val entries: MutableSet<MutableMap.MutableEntry<String, String>>
        get() = form.values.mapNotNull { (k, v) -> v.stringValue?.let { java.util.AbstractMap.SimpleEntry(k, it) } }.toMutableSet()
}

/** 画面の部品を子まで全部(押す対象を id で探す)。 */
fun Screen.allComponents(): List<Component> {
    fun walk(c: Component): List<Component> = listOf(c) + c.children.flatMap(::walk)
    return components.flatMap(::walk)
}

/** ボタンを出すか(新規フォームの削除・読み取りだけの保存は隠す)。Android の components/Leaves.kt と同じ。 */
fun isButtonVisible(action: Action?, form: FormDraft?): Boolean = when (action?.type) {
    ActionType.DELETE_RECORD -> form?.isEditing == true && form.canDelete
    ActionType.SAVE_RECORD -> !(form?.isReadOnly ?: false)
    else -> true
}
