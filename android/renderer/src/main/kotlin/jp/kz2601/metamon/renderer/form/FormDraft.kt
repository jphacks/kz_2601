package jp.kz2601.metamon.renderer.form

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateMapOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.Field
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record

/** 保存ボタンを押した後の結果(iOS の `FormSaveResult`)。 */
enum class FormSaveResult { SAVED, QUEUED_OFFLINE }

/**
 * `screen.entity` を持つ画面が 1 件だけ持つ下書き(iOS の `FormDraft`)。入力部品は `field` をキーに [values] を書く。
 * recordId 付きで開かれたら既存の記録を読んで編集。[values] と印は Compose の状態(書くと描き直す)。
 */
class FormDraft(val entity: Entity, record: Record? = null) {
    var recordId: String? = null
        private set
    var author: String? = null
        private set
    val values: MutableMap<String, JSONValue> = mutableStateMapOf()

    /** 保存で空のまま弾かれた必須の field id(M10)。 */
    var flaggedFields: Set<String> by mutableStateOf(emptySet())
        private set
    var flagCount by mutableIntStateOf(0)
        private set
    /** スクロールしてフォーカスを移す field(M10)。押すたびに null を挟んで同じ欄にも移す。 */
    var attentionField: String? by mutableStateOf(null)
    var saveCount by mutableIntStateOf(0)
        private set
    var lastSaveResult: FormSaveResult? by mutableStateOf(null)
        private set
    var isReadOnly by mutableStateOf(false)
    var canDelete by mutableStateOf(false)

    private var baseline: Map<String, JSONValue> = emptyMap()
    private var baselineBeforeSave: Map<String, JSONValue>? = null

    init {
        reset()
        record?.let(::load)
    }

    val isEditing: Boolean get() = recordId != null

    /** 開いた時(最後に保存した時)の値。型に合わせて正規化済み・空は null。 */
    fun baselineValue(field: String): JSONValue? = baseline[field]
    val hasUnsavedChanges: Boolean get() = !isReadOnly && normalized(values) != baseline

    /** 既存の記録を何も変えずに保存したら書かない(M10)。 */
    val needsWrite: Boolean get() = !isEditing || hasUnsavedChanges

    /** appconfig に送る data(型に合わせて正規化・空と computed は落とす)。 */
    val payload: Map<String, JSONValue> get() = normalized(values)

    fun markClean() {
        baseline = normalized(values)
    }

    fun reset() {
        recordId = null
        author = null
        flaggedFields = emptySet()
        values.clear()
        entity.storedFields.forEach { values[it.id] = FieldCoding.defaultValue(it) }
        markClean()
    }

    fun load(record: Record) {
        recordId = record.id
        author = record.author
        flaggedFields = emptySet()
        entity.storedFields.forEach { values[it.id] = record.data[it.id] ?: FieldCoding.defaultValue(it) }
        markClean()
    }

    /** 新規フォームに外から初期値を入れる(親から子を作る #256)。開いた時の値として扱う。 */
    fun prefill(values: Map<String, JSONValue>) {
        if (values.isEmpty()) return
        this.values.putAll(values)
        markClean()
    }

    /** hidden の入力を出してよいか(iOS の `HiddenFields.canEdit`): 新規・作者無しは出す、作者有りは本人だけ。 */
    fun showsHiddenFields(currentUid: String?): Boolean {
        if (!isEditing) return true
        val a = author?.takeIf { it.isNotEmpty() } ?: return true
        return !currentUid.isNullOrEmpty() && a == currentUid
    }

    /** 部品 [c] が束縛する field が、このドラフトでは隠す hidden field か。 */
    fun hides(c: Component, currentUid: String?): Boolean {
        val fid = c.field ?: return false
        if (c.entity != null && c.entity != entity.id) return false
        if (entity.field(fid)?.hidden != true) return false
        return !showsHiddenFields(currentUid)
    }

    /** 保存の前に呼ぶ。空の必須に印を付け、その名前を返す(空なら保存してよい)。 */
    fun flagMissingRequired(): List<String> {
        val missing = missingRequiredFields
        flaggedFields = missing.map { it.id }.toSet()
        if (missing.isNotEmpty()) flagCount += 1
        return missing.map { it.name }
    }

    /** 印を出す欄か(弾かれた欄のうち、まだ空のもの)。 */
    fun isFlagged(field: String): Boolean {
        if (field !in flaggedFields) return false
        val f = entity.field(field) ?: return false
        return FieldCoding.normalize(values[field] ?: JSONValue.Null, f) == null
    }

    val missingRequiredFields: List<Field>
        get() = entity.storedFields.filter { (it.required ?: false) && FieldCoding.normalize(values[it.id] ?: JSONValue.Null, it) == null }

    /** ホストが書き込んだ後に呼ぶ。 */
    fun markSaved(queuedOffline: Boolean) {
        baselineBeforeSave = baseline
        markClean()
        lastSaveResult = if (queuedOffline) FormSaveResult.QUEUED_OFFLINE else FormSaveResult.SAVED
        saveCount += 1
    }

    /** 送った書き込みをサーバーが断った。「保存しました」を取り消し、入力を未保存に戻す。 */
    fun markSaveRefused() {
        if (lastSaveResult == null) return
        baselineBeforeSave?.let { baseline = it }
        baselineBeforeSave = null
        lastSaveResult = null
    }

    private fun normalized(values: Map<String, JSONValue>): Map<String, JSONValue> = buildMap {
        for (f in entity.storedFields) {
            FieldCoding.normalize(values[f.id] ?: JSONValue.Null, f)?.let { put(f.id, it) }
        }
    }

    companion object {
        /** 保存ボタンを押した後の表示。書き込みが無かった(必須が空・変更なし・確認待ちなど)なら null。 */
        fun saveFeedback(savesBefore: Int?, form: FormDraft?): FormSaveResult? {
            if (form == null || savesBefore == null || form.saveCount == savesBefore) return null
            return form.lastSaveResult
        }
    }
}

/**
 * 親から子を作る(#256): navigate.prefillRef が積み、直後に開いた新規フォームが 1 回だけ消費する(iOS の `FormPrefill`)。
 * 数秒で失効するので古い値が別画面に漏れない。
 */
object FormPrefill {
    private const val TTL_MILLIS = 3_000L
    private var pending: Pair<Map<String, JSONValue>, Long>? = null

    /** テストで時計を差し替える口。 */
    var clock: () -> Long = System::currentTimeMillis

    @Synchronized
    fun set(values: Map<String, JSONValue>) {
        pending = values to clock()
    }

    /** entity にある field だけ返して消費する。 */
    @Synchronized
    fun consume(entity: Entity): Map<String, JSONValue> {
        val (values, at) = pending ?: return emptyMap()
        pending = null
        if (clock() - at >= TTL_MILLIS) return emptyMap()
        return values.filterKeys { entity.field(it) != null }
    }
}
