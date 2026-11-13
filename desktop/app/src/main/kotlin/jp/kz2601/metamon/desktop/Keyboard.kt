package jp.kz2601.metamon.desktop

import androidx.compose.runtime.Composable
import jp.kz2601.metamon.desktop.io.RecordClipboard
import jp.kz2601.metamon.desktop.io.RecordPaste
import java.awt.Toolkit
import java.awt.datatransfer.Transferable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import kotlinx.coroutines.launch
import androidx.compose.runtime.setValue
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.Modifier
import androidx.compose.ui.composed
import androidx.compose.ui.focus.FocusDirection
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.focus.focusProperties
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.foundation.focusGroup
import androidx.compose.foundation.focusable
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.input.key.KeyEvent
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.isAltPressed
import androidx.compose.ui.input.key.isCtrlPressed
import androidx.compose.ui.input.key.isMetaPressed
import androidx.compose.ui.input.key.isShiftPressed
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.onKeyEvent
import androidx.compose.ui.input.key.onPreviewKeyEvent
import androidx.compose.ui.input.key.type
import jp.kz2601.metamon.desktop.debug.DevMenu
import jp.kz2601.metamon.desktop.debug.DevMenuHost
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.desktop.share.ShareLinkDialog
import jp.kz2601.metamon.desktop.share.ShareLinkEntry
import jp.kz2601.metamon.desktop.net.DesktopApiConfig
import jp.kz2601.metamon.desktop.theme.TextZoom
import jp.kz2601.metamon.desktop.theme.TextZoomState
import jp.kz2601.metamon.desktop.gridedit.GridEditDraft
import jp.kz2601.metamon.desktop.gridedit.GridEditLayer
import jp.kz2601.metamon.desktop.gridedit.gridEditDraft
import jp.kz2601.metamon.desktop.usage.UsageDialog
import jp.kz2601.metamon.desktop.usage.UsageLoader
import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.desktop.io.ExportClipboard
import jp.kz2601.metamon.desktop.store.RetentionPrompts
import jp.kz2601.metamon.desktop.store.RetentionReviewDialog
import jp.kz2601.metamon.desktop.voice.PromptTipsDialog
import jp.kz2601.metamon.desktop.voice.PromptTipsText
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.Presentation
import jp.kz2601.metamon.dsl.Screen
import jp.kz2601.metamon.dsl.screen
import jp.kz2601.metamon.renderer.form.FormDraft
import jp.kz2601.metamon.desktop.onboarding.OnboardingDialog
import jp.kz2601.metamon.services.PlanUsageSource

// キーボードだけで使う(#1765): ショートカット・コマンドパレット(CommandPalette.kt)・F1 の一覧。

/** キーボードまわりの文言(i18n の desktop_keys_*)。 */
object KeyboardText {
    val PALETTE get() = Strings["desktop_keys_palette"]
    val NEW get() = Strings["desktop_keys_new"]
    val SAVE get() = Strings["desktop_keys_save"]
    val SEARCH get() = Strings["desktop_keys_search"]
    val DELETE get() = Strings["desktop_keys_delete"]
    val CLOSE get() = Strings["desktop_keys_close"]
    val HELP get() = Strings["desktop_keys_help"]
    val PRINT get() = Strings["desktop_print"]
    val PREVIEW get() = Strings["desktop_keys_preview"]
    val NO_PREVIEW get() = Strings["desktop_preview_none"]
    val DELETE_CONFIRM get() = Strings["desktop_keys_delete_confirm"]
    val UNDO get() = Strings["feedback_undo"]
    val NO_NEW get() = Strings["desktop_keys_no_new"]
    val PASTE_NONE get() = Strings["desktop_paste_record_none"]
    val NO_SAVE get() = Strings["desktop_keys_no_save"]
    val PALETTE_PLACEHOLDER get() = Strings["desktop_keys_palette_placeholder"]
    val PALETTE_EMPTY get() = Strings["desktop_keys_palette_empty"]
    val KIND_ACTION get() = Strings["desktop_keys_kind_action"]
    val KIND_SCREEN get() = Strings["desktop_keys_kind_screen"]
    val KIND_APP get() = Strings["desktop_keys_kind_app"]
    val HELP_CLOSE get() = Strings["desktop_close"]
    val HELP_TAB get() = Strings["desktop_keys_help_tab"]
    val NEW_WINDOW get() = Strings["desktop_tray_new_window"]
    val OPEN_RECORD_WINDOW get() = Strings["desktop_window_open_record"]
    val WIPE get() = Strings["desktop_wipe"]
    val WIPE_CONFIRM get() = Strings["desktop_wipe_confirm"]
    val WIPE_CONFIRM_BUTTON get() = Strings["desktop_wipe_confirm_button"]
    val WIPE_DONE get() = Strings["desktop_wipe_done"]
    val ZOOM_IN get() = Strings["desktop_keys_zoom_in"]
    val ZOOM_OUT get() = Strings["desktop_keys_zoom_out"]
    val ZOOM_RESET get() = Strings["desktop_keys_zoom_reset"]
}

/** キーの決まりを分ける OS。Windows・Linux は Ctrl、macOS は ⌘。 */
enum class HostOs {
    WINDOWS, LINUX, MAC;

    companion object {
        fun of(osName: String): HostOs = osName.lowercase().let {
            when {
                it.startsWith("mac") || it.contains("darwin") -> MAC
                it.startsWith("windows") -> WINDOWS
                else -> LINUX
            }
        }

        val current: HostOs by lazy { of(System.getProperty("os.name").orEmpty()) }
    }
}

/** ショートカット。[bubble] は入力欄に先に渡す物(Esc・Delete・F1)。他は入力欄より先に取る。 */
enum class Shortcut(private val text: () -> String, val bubble: Boolean = false) {
    PALETTE({ KeyboardText.PALETTE }),
    NEW({ KeyboardText.NEW }),
    SAVE({ KeyboardText.SAVE }),
    SEARCH({ KeyboardText.SEARCH }),
    DELETE({ KeyboardText.DELETE }, bubble = true),
    CLOSE({ KeyboardText.CLOSE }, bubble = true),
    PRINT({ KeyboardText.PRINT }),
    HELP({ KeyboardText.HELP }, bubble = true),

    /** 写真を大きく見る(PhotoPreview.kt)。Space は入力欄・ボタンが先に受け、何も選んでいない時だけ開く。 */
    PREVIEW({ KeyboardText.PREVIEW }, bubble = true),

    /** 一覧・表で消した行を戻す(#3351)。入力欄の Ctrl+Z(文字の取り消し)を先に通す。戻す物が無ければ流す。 */
    UNDO({ KeyboardText.UNDO }, bubble = true),
    /** 画面全体の文字の大きさ(#2208・theme/TextZoom.kt)。 */
    ZOOM_IN({ KeyboardText.ZOOM_IN }),
    ZOOM_OUT({ KeyboardText.ZOOM_OUT }),
    ZOOM_RESET({ KeyboardText.ZOOM_RESET }),
    ;

    val title: String get() = text()
}

/** 押したキー(KeyEvent から作る・テストでは直に作る)。 */
data class KeyPress(
    val key: Key,
    val ctrl: Boolean = false,
    val meta: Boolean = false,
    val shift: Boolean = false,
    val alt: Boolean = false,
) {
    companion object {
        fun of(e: KeyEvent): KeyPress? =
            if (e.type != KeyEventType.KeyDown) null else KeyPress(e.key, e.isCtrlPressed, e.isMetaPressed, e.isShiftPressed, e.isAltPressed)
    }
}

/** キーの組み。[primary] は Windows・Linux の Ctrl、macOS の ⌘。 */
data class KeyChord(val key: Key, val primary: Boolean = false, val shift: Boolean = false)

/** OS ごとのキー割り当て。Ctrl+P は印刷(Print.kt)なので、パレットは Ctrl+K(と Ctrl+Shift+P)。 */
object Keymap {
    fun chords(s: Shortcut, os: HostOs): List<KeyChord> = when (s) {
        Shortcut.PALETTE -> listOf(KeyChord(Key.K, primary = true), KeyChord(Key.P, primary = true, shift = true))
        Shortcut.NEW -> listOf(KeyChord(Key.N, primary = true))
        Shortcut.SAVE -> listOf(KeyChord(Key.S, primary = true))
        Shortcut.SEARCH -> listOf(KeyChord(Key.F, primary = true))
        Shortcut.PRINT -> listOf(KeyChord(Key.P, primary = true))
        Shortcut.CLOSE -> listOf(KeyChord(Key.Escape))
        Shortcut.HELP -> listOf(KeyChord(Key.F1), KeyChord(Key.Slash, primary = true))
        Shortcut.PREVIEW -> listOf(KeyChord(Key.Spacebar))
        Shortcut.UNDO -> listOf(KeyChord(Key.Z, primary = true))
        // macOS のキーボードの delete は Backspace。項目の削除は ⌘⌫(Finder と同じ)。fn+delete(前方削除)も受ける
        Shortcut.DELETE -> if (os == HostOs.MAC) listOf(KeyChord(Key.Backspace, primary = true), KeyChord(Key.Delete)) else listOf(KeyChord(Key.Delete))
        Shortcut.ZOOM_IN -> listOf(KeyChord(Key.Plus, primary = true))
        Shortcut.ZOOM_OUT -> listOf(KeyChord(Key.Minus, primary = true))
        Shortcut.ZOOM_RESET -> listOf(KeyChord(Key.Zero, primary = true))
    }

    /**
     * 一覧には出さないが受ける組み。「+」は配列で押し方が違う: US は Shift+=(= だけでも受ける)、JIS は Shift+;。テンキーも受ける。
     */
    private fun aliases(s: Shortcut): List<KeyChord> = when (s) {
        Shortcut.ZOOM_IN -> listOf(
            KeyChord(Key.Plus, primary = true, shift = true),
            KeyChord(Key.Equals, primary = true),
            KeyChord(Key.Equals, primary = true, shift = true),
            KeyChord(Key.Semicolon, primary = true, shift = true),
            KeyChord(Key.NumPadAdd, primary = true),
        )
        Shortcut.ZOOM_OUT -> listOf(KeyChord(Key.Minus, primary = true, shift = true), KeyChord(Key.NumPadSubtract, primary = true))
        Shortcut.ZOOM_RESET -> listOf(KeyChord(Key.NumPad0, primary = true))
        else -> emptyList()
    }

    fun matches(c: KeyChord, p: KeyPress, os: HostOs): Boolean {
        if (c.key != p.key || p.alt || c.shift != p.shift) return false
        val primary = if (os == HostOs.MAC) p.meta else p.ctrl
        val other = if (os == HostOs.MAC) p.ctrl else p.meta
        return primary == c.primary && !other
    }

    /** 押したキーのショートカット。印刷も返す(処理は Print.kt)。 */
    fun match(p: KeyPress, os: HostOs): Shortcut? = Shortcut.entries.firstOrNull { s -> (chords(s, os) + aliases(s)).any { matches(it, p, os) } }

    /** 一覧・パレットに出す表記(最初の組み)。Windows・Linux は「Ctrl+Shift+P」、macOS は「⇧⌘P」。 */
    fun label(s: Shortcut, os: HostOs): String = label(chords(s, os).first(), os)

    fun label(c: KeyChord, os: HostOs): String {
        val k = keyName(c.key, os)
        return if (os == HostOs.MAC) {
            (if (c.shift) "⇧" else "") + (if (c.primary) "⌘" else "") + k
        } else {
            listOfNotNull("Ctrl".takeIf { c.primary }, "Shift".takeIf { c.shift }, k).joinToString("+")
        }
    }

    internal fun keyName(k: Key, os: HostOs): String = when (k) {
        Key.Escape -> "Esc"
        Key.Delete -> if (os == HostOs.MAC) "⌦" else "Delete"
        Key.Backspace -> if (os == HostOs.MAC) "⌫" else "Backspace"
        Key.F1 -> "F1"
        Key.Spacebar -> "Space"
        Key.Slash -> "/"
        Key.K -> "K"
        Key.N -> "N"
        Key.P -> "P"
        Key.S -> "S"
        Key.F -> "F"
        Key.Z -> "Z"
        Key.Plus -> "+"
        Key.Minus -> "-"
        Key.Zero -> "0"
        else -> k.toString()
    }
}

/** 一番上の画面でショートカットが押すアクション(画面に無い物は作らない)。 */
object ScreenShortcuts {
    private val ADD_WORDS = listOf("追加", "新規", "作る", "add", "new", "create")

    /** 新規: toolbar・ボタンのうち、フォームの画面(entity あり)へ移る物。「追加」「plus」らしい物を先に。 */
    fun newAction(dsl: AppDSL, screen: Screen): Action? {
        val candidates = buttons(screen).filter { (_, _, a) ->
            a.type == ActionType.NAVIGATE && a.screen?.let { dsl.screen(it)?.entity } != null
        }
        return (candidates.firstOrNull { (label, icon, _) -> icon == "plus" || ADD_WORDS.any { label.contains(it, ignoreCase = true) } } ?: candidates.firstOrNull())?.third
    }

    /** 保存: 画面の保存ボタン。無くてもフォームの画面なら保存する。 */
    fun saveAction(screen: Screen, form: FormDraft?): Action? {
        if (form != null && form.isReadOnly) return null
        buttons(screen).firstOrNull { it.third.type == ActionType.SAVE_RECORD }?.let { return it.third }
        return form?.let { Action(type = ActionType.SAVE_RECORD, entity = it.entity.id) }
    }

    /** 削除: 画面に見えている削除ボタン。確認の無い物には確認を付ける。 */
    fun deleteAction(screen: Screen, form: FormDraft?): Action? {
        val a = buttons(screen).map { it.third }.firstOrNull { it.type == ActionType.DELETE_RECORD && isButtonVisible(it, form) } ?: return null
        return withConfirm(a)
    }

    fun withConfirm(a: Action): Action = if (a.confirmMessage == null) a.copy(confirmMessage = KeyboardText.DELETE_CONFIRM) else a

    fun hasSearch(screen: Screen): Boolean = screen.allComponents().any { it.type == ComponentType.SEARCH_BAR }

    /** (ラベル, アイコン, アクション)。toolbar → 部品(子まで)の順。 */
    fun buttons(screen: Screen): List<Triple<String, String?, Action>> =
        screen.toolbar.map { Triple(it.label, it.icon, it.action) } +
            screen.allComponents().filter { it.type == ComponentType.BUTTON }.mapNotNull { c ->
                c.action?.let { Triple(c.label ?: c.title ?: c.text.orEmpty(), c.icon, it) }
            }
}

/** Ctrl+F の行き先: 今描いている searchBar。後から描いた物(sheet の中)を先に。ウィンドウごとに 1 つ(#1763)。 */
class SearchTargets {
    private val targets = mutableListOf<FocusRequester>()

    fun focusTop(): Boolean = synchronized(targets) { targets.toList() }.asReversed().any { runCatching { it.requestFocus() }.isSuccess }

    @Composable
    fun Register(r: FocusRequester) {
        DisposableEffect(r) {
            synchronized(targets) { targets += r }
            onDispose { synchronized(targets) { targets -= r } }
        }
    }
}

/** 今のウィンドウの Ctrl+F の行き先(DesktopApp が渡す)。 */
val LocalSearchTargets = staticCompositionLocalOf { SearchTargets() }

/** searchBar に付ける(Ctrl+F で入る)。 */
fun Modifier.searchShortcutTarget(): Modifier = composed {
    val r = remember { FocusRequester() }
    LocalSearchTargets.current.Register(r)
    focusRequester(r)
}

/**
 * デスクトップ版のキーボード操作。DesktopApp の一番外に [modifier] を付け、[Overlay] でパレットと一覧を出す。
 * 何も選ばれていない時もキーが届くよう、一番外がフォーカスを持つ。
 */
class DesktopKeyboard(
    val dsl: AppDSL,
    val session: DesktopSession,
    val os: HostOs = HostOs.current,
) {
    var dispatch: (Action, ActionContext) -> Unit = { a, ctx -> session.dispatch(a, ctx) }
    var print: () -> Unit = {}
    var openApp: ((String) -> Unit)? = null

    /** 記録の詳細を別のウィンドウで開く(#1763)。(画面, recordId)。 */
    var openRecord: ((String, String) -> Unit)? = null

    /** 「この端末のデータを消す」(#1887)。全アプリの分を消す口(AppHub.wipeLocalData)。null ならパレットに出さない。 */
    var wipeLocalData: (suspend () -> Unit)? = null

    /** 「診断の情報をコピー」(#2200・crash/Diagnostics.kt)。null ならパレット・F1 の一覧に出さない。 */
    var copyDiagnostics: (() -> Unit)? = null
    /** 並びを直す編集の保存(#3207)。null ならパレットに出さない。 */
    var saveLayout: ((GridEditDraft) -> Unit)? = null

    /** 並びを直す編集の最中(その画面の下書き)。 */
    var gridEdit by mutableStateOf<GridEditDraft?>(null)

    /** 消す前の確認を出している。 */
    var wipeConfirmOpen by mutableStateOf(false)

    /** 接続先(開発用メニューを出すかを決める)。 */
    var apiConfig: DesktopApiConfig = DesktopApiConfig.fromEnvironment()

    /** 開発用メニューの「今の環境」の中身。null なら閉じている。 */
    var devInfo by mutableStateOf<Pair<String, List<String>>?>(null)

    /** 開発用メニュー(debug/DevMenu.kt)から触る口。 */
    val devHost: DevMenuHost = object : DevMenuHost {
        override val config: DesktopApiConfig get() = apiConfig
        override val pendingCount: Int? get() = session.persistence?.pending
        override val canWipe: Boolean get() = wipeLocalData != null
        override fun showInfo(title: String, lines: List<String>) { devInfo = title to lines }
        override fun askWipe() { wipeConfirmOpen = true }
    }
    /** アカウントの画面(AccountScreen.kt・#2374・#2505)。パレット・トレイから開く。開いている間のキーは画面が自分で受ける。 */
    var accountOpen by mutableStateOf(false)

    /** このウィンドウの検索欄(Ctrl+F は他のウィンドウに入らない)。 */
    val searchTargets = SearchTargets()

    var paletteOpen by mutableStateOf(false)
    var helpOpen by mutableStateOf(false)

    /** 写真のプレビュー(#1850 のデスクトップ版)。 */
    val photos = PhotoPreview()

    /** 一番外そのものがフォーカスを持つ(入力欄・ボタンなど中の部品を選んでいない)。Space で写真を開くのはこの時だけ。 */
    internal var rootFocused = false

    /** 画面全体の文字の倍率(#2208)。全部のウィンドウで 1 つ。 */
    var textZoom: TextZoomState = TextZoom.app

    /** サードパーティのライセンス(Licenses.kt)。F1 の一覧とパレットから開く。 */
    var licensesOpen by mutableStateOf(false)
    var a11yBridgeOpen by mutableStateOf(false)

    /** 共有のリンク(share/ShareLink.kt・#2503)。サーバーのアプリを開いている時に入れる。null ならパレットに出さない。 */
    var share: ShareLinkEntry? = null

    private val shareOpen: Boolean get() = share?.open == true

    /** 設定の「使っている量」(#2814)。サインイン中だけパレットに出す。 */
    var usage: UsageLoader? = null
    var usageOpen by mutableStateOf(false)

    /** 頼み方のコツ(voice/PromptTipsDialog.kt)。F1 の一覧とパレットから開く。 */
    var tipsOpen by mutableStateOf(false)

    /** 設定の「自動で消える記録」の見直しの一覧(store/RetentionPrompt.kt)。パレットから開く。 */
    var retentionReviewOpen by mutableStateOf(false)
    private var retentionTick by mutableIntStateOf(0)

    /** 初回の流れ(#3201)。起動時に 1 回・F1 の一覧とパレットの「使い方を見る」から開く。 */
    var onboardingOpen by mutableStateOf(false)

    /** 初回の流れを閉じた(最後まで見た = true・あとで見る / Esc = false)。 */
    var onOnboardingFinished: (Boolean) -> Unit = {}

    val root = FocusRequester()
    val modal = FocusRequester()

    private val topScreen: Screen? get() = dsl.screen(session.top.screen)
    private fun ctx() = session.top.let { ActionContext(it, session.form(it)) }

    fun handle(e: KeyEvent, bubble: Boolean): Boolean {
        // 離したことも見る(Space の押しっぱなしを見分けるため)
        val repeat = e.isAutoRepeatSpace()
        return KeyPress.of(e)?.let { handle(it, bubble, repeat) } ?: false
    }

    /** [repeat] は Space の押しっぱなしの 2 回目以降(開く・閉じるをしない)。 */
    fun handle(p: KeyPress, bubble: Boolean, repeat: Boolean = false): Boolean {
        if (accountOpen) return false
        if (photos.isOpen) {
            // ダイアログが受けなかった閉じるキー(フォーカスがダイアログの外にある時)はここで閉じる。他は流す
            if (!bubble || PhotoPreview.keyOf(p) != PreviewKey.CLOSE) return false
            if (!repeat) closePreview()
            return true
        }
        if (paletteOpen || helpOpen || licensesOpen || usageOpen || wipeConfirmOpen || shareOpen || onboardingOpen || gridEdit != null || a11yBridgeOpen || devInfo != null) return false // パレット・一覧・確認・並びの編集が自分で受ける
        // 項目つきでコピーした記録の貼り付け。入力欄などを選んでいる時は普通の貼り付けに任せる
        if (isPasteKey(p)) return !bubble && rootFocused && pasteRecord()
        val s = Keymap.match(p, os) ?: return false
        if (s == Shortcut.PRINT || s.bubble != bubble) return false // 印刷は Print.kt の Ctrl+P
        // Space は何も選んでいない時だけ。見せる写真が無ければ流す(知らせも出さない)
        if (s == Shortcut.PREVIEW) return rootFocused && !repeat && openPreview()
        return run(s)
    }

    /** クリップボードの中身(テストで差し替える)。 */
    var clipboard: () -> Transferable? = { runCatching { Toolkit.getDefaultToolkit().systemClipboard.getContents(null) }.getOrNull() }

    private fun isPasteKey(p: KeyPress): Boolean =
        p.key == Key.V && !p.shift && !p.alt && (if (os == HostOs.MAC) p.meta && !p.ctrl else p.ctrl && !p.meta)

    /**
     * Ctrl+V(macOS は ⌘V): 項目つきでコピーした記録が入っていれば、一番上のフォームの項目名が合う欄に値を入れる。
     * 入った件数と入らなかった件数を知らせる。コピーした記録が無い・書けないフォームなら false(他へ流す)。
     */
    fun pasteRecord(): Boolean {
        val form = session.form(session.top) ?: return false
        if (form.isReadOnly) return false
        val payload = clipboard()?.let(RecordClipboard::read) ?: return false
        val r = RecordPaste.paste(payload, form.entity, session.persistence?.appId.orEmpty())
        r.values.forEach { (field, v) -> form.values[field] = v }
        session.message = when {
            r.values.isEmpty() -> KeyboardText.PASTE_NONE
            r.unmatched.isEmpty() -> Strings.format("desktop_paste_record_done", r.values.size)
            else -> Strings.format("desktop_paste_record_partial", r.values.size, r.unmatched.size)
        }
        return true
    }

    /** 一番上の画面の記録の写真を開く([start] から)。見せる物が無ければ false。 */
    fun openPreview(start: String? = null): Boolean = photos.open(PhotoPreviewSources.of(session.form(session.top)), start)

    /** 写真の部品を押した: その写真から開く。 */
    fun openPhoto(src: String, name: String): Boolean = photos.openAt(PhotoPreviewSources.of(session.form(session.top)), src, name)

    /** プレビューを閉じてフォーカスを戻す(sheet の中から開いた時は sheet へ)。 */
    fun closePreview() {
        photos.close()
        focusAfterNavigation(session.nav.sheet != null || session.nav.fullScreen != null)
    }

    /** ショートカット 1 つ。この画面で使えなければ false(キーは他へ流す)。 */
    fun run(s: Shortcut): Boolean {
        zoom(s)?.let { v ->
            session.message = Strings.format("desktop_text_zoom_now", TextZoom.percent(v))
            return true
        }
        val screen = topScreen ?: return false
        val form = session.form(session.top)
        when (s) {
            Shortcut.PALETTE -> paletteOpen = true
            Shortcut.HELP -> helpOpen = true
            Shortcut.PRINT -> print()
            Shortcut.NEW -> ScreenShortcuts.newAction(dsl, screen)?.let { dispatch(it, ctx()) } ?: run { session.message = KeyboardText.NO_NEW }
            Shortcut.SAVE -> ScreenShortcuts.saveAction(screen, form)?.let { dispatch(it, ctx()) } ?: run { session.message = KeyboardText.NO_SAVE }
            Shortcut.SEARCH -> if (!searchTargets.focusTop()) paletteOpen = true
            Shortcut.DELETE -> dispatch(ScreenShortcuts.deleteAction(screen, form) ?: return false, ctx())
            Shortcut.CLOSE -> if (session.canClose(session.top)) session.back(session.top) else return false
            Shortcut.PREVIEW -> if (!openPreview()) session.message = KeyboardText.NO_PREVIEW
            Shortcut.UNDO -> return session.undoDelete()
            Shortcut.ZOOM_IN, Shortcut.ZOOM_OUT, Shortcut.ZOOM_RESET -> Unit // 上の zoom で済んでいる
        }
        return true
    }

    /** 文字の大きさのショートカットなら倍率を変えて新しい値を返す。他は null。 */
    private fun zoom(s: Shortcut): Float? = when (s) {
        Shortcut.ZOOM_IN -> textZoom.zoomIn()
        Shortcut.ZOOM_OUT -> textZoom.zoomOut()
        Shortcut.ZOOM_RESET -> textZoom.reset()
        else -> null
    }

    /** 画面へ移る(タブはタブを選ぶ・他は push)。 */
    fun openScreen(id: String) {
        if (dsl.app.tabs.any { it.screen == id }) {
            session.selectTab(id)
            return
        }
        dispatch(Action(type = ActionType.NAVIGATE, screen = id, presentation = Presentation.PUSH), ctx())
    }

    /** 並びを直す編集を始める(メインの層の、動かせる部品がある画面だけ)。 */
    fun startGridEdit() {
        val draft = if (session.nav.sheet == null && session.nav.fullScreen == null) dsl.gridEditDraft(session.top.screen) else null
        if (draft == null) {
            session.message = Strings["desktop_grid_edit_unavailable"]
            return
        }
        gridEdit = draft
    }

    /** パレットの中身: ショートカット → この画面のボタン → 画面 → アプリ。 */
    fun commands(): List<PaletteCommand> {
        val screen = topScreen
        val form = session.form(session.top)
        val shortcuts = Shortcut.entries.filter { it != Shortcut.PALETTE }.map { s ->
            PaletteCommand("shortcut:${s.name}", s.title, PaletteCommand.Kind.ACTION, hint = Keymap.label(s, os)) { run(s) }
        }
        val buttons = screen?.let { ScreenShortcuts.buttons(it) }.orEmpty()
            .filter { (label, _, a) -> label.isNotBlank() && isButtonVisible(a, form) }
            .mapIndexed { i, (label, _, a) ->
                val action = if (a.type == ActionType.DELETE_RECORD) ScreenShortcuts.withConfirm(a) else a
                PaletteCommand("button:$i", label, PaletteCommand.Kind.ACTION) { dispatch(action, ctx()) }
            }
        val screens = dsl.screens.map { sc ->
            PaletteCommand("screen:${sc.id}", sc.title.ifEmpty { sc.id }, PaletteCommand.Kind.SCREEN, keywords = listOf(sc.id)) { openScreen(sc.id) }
        }
        // アプリは別のウィンドウで開く(開いていればそれを前に出す)
        val apps = openApp?.let { open ->
            BundledApps.all().map { (id, name) ->
                PaletteCommand("app:$id", name, PaletteCommand.Kind.APP, keywords = listOf(id, KeyboardText.NEW_WINDOW)) { open(id) }
            }
        }.orEmpty()
        // 今開いている記録を別のウィンドウに(詳細のウィンドウの根では出さない)
        val top = session.top
        val record = openRecord?.takeIf { top.recordId != null && top != session.detailRoot }?.let { open ->
            listOf(PaletteCommand("window:record", KeyboardText.OPEN_RECORD_WINDOW, PaletteCommand.Kind.ACTION) { open(top.screen, top.recordId!!) })
        }.orEmpty()
        // サインアウトの画面(#1749)ができるまでの口。押すと確認を出す(すぐには消さない)
        val wipe = wipeLocalData?.let {
            listOf(PaletteCommand("data:wipe", KeyboardText.WIPE, PaletteCommand.Kind.ACTION, keywords = listOf("wipe", "sign out")) { wipeConfirmOpen = true })
        }.orEmpty()
        val diagnostics = copyDiagnostics?.let { copy ->
            listOf(PaletteCommand("about:diagnostics", Strings["desktop_diagnostics_copy"], PaletteCommand.Kind.ACTION, keywords = listOf("diagnostics", "support", "version")) { copy() })
        }.orEmpty()
        val licenses = PaletteCommand(
            "about:licenses", LicensesText.TITLE, PaletteCommand.Kind.ACTION,
            keywords = listOf("licenses", "third-party", "oss", "notice", LicensesText.LICENSE),
        ) { licensesOpen = true }
        val account = PaletteCommand(
            "about:account", Strings["account_menu"], PaletteCommand.Kind.ACTION,
            keywords = listOf("account", "sign in", "sign out", "login", "terms", "privacy", "contact", Strings["legal_terms"], Strings["legal_privacy_policy"], Strings["settings_contact"]),
        ) { accountOpen = true }
        val shareLink = share?.let { listOf(it.command()) }.orEmpty()
        val usage = usage?.let {
            listOf(PaletteCommand("account:usage", Strings["settings_usage_header"], PaletteCommand.Kind.ACTION, keywords = listOf("usage", "plan", "limit")) { usageOpen = true })
        }.orEmpty()
        val tips = PaletteCommand(
            "help:prompt_tips", PromptTipsText.TITLE, PaletteCommand.Kind.ACTION,
            keywords = listOf("tips", "prompt", "examples", "help"),
        ) { tipsOpen = true }
        val retention = session.takeIf { retentionRows().isNotEmpty() }?.let {
            listOf(
                PaletteCommand(
                    "retention:review", Strings["desktop_retention_review"], PaletteCommand.Kind.ACTION,
                    keywords = listOf("retention", "auto delete", Strings["retention_settings_header"]),
                ) { retentionReviewOpen = true },
            )
        }.orEmpty()
        val onboarding = PaletteCommand(
            "help:onboarding", Strings["onboarding_replay"], PaletteCommand.Kind.ACTION,
            keywords = listOf("onboarding", "tutorial", "how to use", Strings["desktop_onboarding_title"]),
        ) { onboardingOpen = true }
        val layout = saveLayout?.let {
            listOf(PaletteCommand("layout:edit", Strings["desktop_grid_edit"], PaletteCommand.Kind.ACTION, keywords = listOf("layout", "grid", "area")) { startGridEdit() })
        }.orEmpty()
        // Windows でナレーターが動いていて Access Bridge が無効の時だけ出す(#2209)
        val bridge = if (AccessBridgeAdvice.needed()) {
            listOf(PaletteCommand("about:a11y-bridge", Strings["desktop_a11y_bridge_command"], PaletteCommand.Kind.ACTION, keywords = listOf("narrator", "screen reader", "jabswitch")) { a11yBridgeOpen = true })
        } else {
            emptyList()
        }
        // stg の時だけ・配布物では出ない
        val dev = DevMenu.commands(devHost)
        return shortcuts + buttons + record + layout + screens + apps + usage + wipe + account + shareLink + retention + tips + onboarding + diagnostics + licenses + bridge + dev
    }

    /** 今のアプリの保存期間のある entity(アプリ・entity・日数・確認済みか)。端末に残さない時は空。 */
    fun retentionRows(): List<RetentionPrompts.Row> {
        val runner = session.retention ?: return emptyList()
        val appId = session.records.persistence?.appId ?: return emptyList()
        return RetentionPrompts.rows(appId, dsl.app.name, runner, session.models)
    }

    /** 確認で「消す」を押した: 全アプリの端末のデータを消し、このウィンドウを最初の画面に戻す。 */
    suspend fun confirmWipe() {
        wipeConfirmOpen = false
        val wipe = wipeLocalData ?: return
        wipe()
        session.followWipe()
        session.message = KeyboardText.WIPE_DONE
    }

    /** 一番外に付ける: Ctrl の組みは入力欄より先に、Esc・Delete・F1 は入力欄の後に受ける。 */
    val modifier: Modifier = Modifier
        .onPreviewKeyEvent { handle(it, bubble = false) }
        .onKeyEvent { handle(it, bubble = true) }
        .focusRequester(root)
        .onFocusChanged { rootFocused = it.isFocused }
        .focusable()

    /**
     * sheet・fullScreen が開いている間の下の画面(タブも含む): Tab で入らず、読み上げにも出さない。
     * 中にフォーカスが残っていたら [focusAfterNavigation] がモーダルへ移す。
     */
    fun inert(blocked: Boolean): Modifier =
        if (blocked) Modifier.focusProperties { onEnter = { cancelFocusChange() } }.focusGroup().clearAndSetSemantics { } else Modifier

    /** sheet・fullScreen の枠。開いたら中の最初の部品へ入る(入れる物が無ければ一番外に戻す)。 */
    val modalModifier: Modifier get() = Modifier.focusRequester(modal).focusGroup()

    fun focusAfterNavigation(modalOpen: Boolean) {
        if (paletteOpen || helpOpen || licensesOpen || usageOpen || shareOpen || tipsOpen || retentionReviewOpen || onboardingOpen || devInfo != null) return
        if (modalOpen && runCatching { modal.requestFocus(FocusDirection.Enter) }.getOrDefault(false)) return
        runCatching { root.requestFocus() }
    }

    /** パレット・一覧。DesktopApp の画面の上(Surface の中の最後)に置く。 */
    @Composable
    fun Overlay() {
        val modalOpen = session.nav.sheet != null || session.nav.fullScreen != null
        LaunchedEffect(session.nav) {
            photos.close() // 画面が変わったら前の記録の写真は閉じる
            focusAfterNavigation(modalOpen)
        }
        gridEdit?.let { draft ->
            val close = {
                gridEdit = null
                runCatching { root.requestFocus() }
            }
            GridEditLayer(draft, Strings["desktop_grid_edit"], onSave = { d -> close(); saveLayout?.invoke(d) }, onCancel = { close() })
        }
        if (photos.isOpen) PhotoPreviewDialog(photos, onMessage = { session.message = it }, onDismiss = ::closePreview)
        if (paletteOpen) {
            CommandPalette(remember(session.nav, session.store) { commands() }, os) {
                paletteOpen = false
                runCatching { root.requestFocus() }
            }
        }
        if (helpOpen) {
            ShortcutHelp(os, onLicenses = { helpOpen = false; licensesOpen = true }, onTips = { helpOpen = false; tipsOpen = true }, onOnboarding = { helpOpen = false; onboardingOpen = true }, onCopyDiagnostics = copyDiagnostics) {
                helpOpen = false
                runCatching { root.requestFocus() }
            }
        }
        LaunchedEffect(Unit) { if (AccessBridgeAdvice.shouldAutoShow()) a11yBridgeOpen = true }
        if (a11yBridgeOpen) {
            AlertDialog(
                onDismissRequest = { a11yBridgeOpen = false },
                title = { Text(Strings["desktop_a11y_bridge_title"]) },
                text = { Text(Strings["desktop_a11y_bridge_body"]) },
                confirmButton = { TextButton(onClick = { a11yBridgeOpen = false }) { Text(KeyboardText.HELP_CLOSE) } },
            )
        }
        if (tipsOpen) {
            PromptTipsDialog(
                onCopy = { ExportClipboard.put(it) },
                onMessage = { session.message = it },
                onDismiss = {
                    tipsOpen = false
                    runCatching { root.requestFocus() }
                },
            )
        }
        if (retentionReviewOpen) {
            val runner = session.retention
            val appId = session.records.persistence?.appId
            val rows = remember(retentionTick, session.store) { retentionRows() }
            RetentionReviewDialog(
                rows,
                onConfirm = { r ->
                    val entity = session.entityModel(r.entityId)
                    if (runner != null && appId != null && entity != null) {
                        RetentionPrompts.confirm(runner, appId, entity)
                        retentionTick++
                    }
                },
                onDismiss = {
                    retentionReviewOpen = false
                    runCatching { root.requestFocus() }
                },
            )
        }
        if (onboardingOpen) {
            OnboardingDialog(os) { completed ->
                onboardingOpen = false
                onOnboardingFinished(completed)
                // あとで見る: どこから見直せるかを一言で
                if (!completed) session.message = Strings.format("desktop_onboarding_later_hint", Keymap.label(Shortcut.HELP, os))
                runCatching { root.requestFocus() }
            }
        }
        if (licensesOpen) {
            LicensesDialog {
                licensesOpen = false
                runCatching { root.requestFocus() }
            }
        }
        share?.takeIf { it.open }?.let { entry ->
            ShareLinkDialog(entry) {
                entry.open = false
                runCatching { root.requestFocus() }
            }
        }
        devInfo?.let { (title, lines) ->
            val dismiss = {
                devInfo = null
                runCatching { root.requestFocus() }
            }
            AlertDialog(
                onDismissRequest = { dismiss() },
                title = { Text(title) },
                text = { Text(lines.joinToString("\n")) },
                confirmButton = { TextButton(onClick = { dismiss() }) { Text(KeyboardText.HELP_CLOSE) } },
            )
        }
        usage?.takeIf { usageOpen }?.let { loader ->
            UsageDialog(loader) {
                usageOpen = false
                runCatching { root.requestFocus() }
            }
        }
        if (wipeConfirmOpen) {
            val scope = rememberCoroutineScope()
            val dismiss = {
                wipeConfirmOpen = false
                runCatching { root.requestFocus() }
            }
            AlertDialog(
                onDismissRequest = { dismiss() },
                title = { Text(KeyboardText.WIPE) },
                text = { Text(KeyboardText.WIPE_CONFIRM) },
                confirmButton = { TextButton(onClick = { scope.launch { confirmWipe() } }) { Text(KeyboardText.WIPE_CONFIRM_BUTTON) } },
                dismissButton = { TextButton(onClick = { dismiss() }) { Text(Strings["inputs_cancel"]) } },
            )
        }
        if (accountOpen) {
            AccountDialog(onLicenses = { accountOpen = false; licensesOpen = true }) {
                accountOpen = false
                runCatching { root.requestFocus() }
            }
        }
    }
}

@Composable
fun rememberDesktopKeyboard(
    dsl: AppDSL,
    session: DesktopSession,
    dispatch: (Action, ActionContext) -> Unit,
    print: () -> Unit,
    openApp: ((String) -> Unit)?,
    openRecord: ((String, String) -> Unit)? = null,
    wipeLocalData: (suspend () -> Unit)? = null,
    usage: PlanUsageSource? = null,
    saveLayout: ((GridEditDraft) -> Unit)? = null,
): DesktopKeyboard = remember(dsl, session) { DesktopKeyboard(dsl, session) }.also {
    it.dispatch = dispatch
    it.print = print
    it.openApp = openApp
    it.openRecord = openRecord
    it.wipeLocalData = wipeLocalData
    if (it.usage == null && usage != null) it.usage = UsageLoader(usage)
    it.saveLayout = saveLayout
}
