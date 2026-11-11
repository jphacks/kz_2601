package jp.kz2601.metamon.desktop

import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.composed
import androidx.compose.ui.focus.FocusDirection
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.focus.focusProperties
import androidx.compose.ui.focus.focusRequester
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
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.Presentation
import jp.kz2601.metamon.dsl.Screen
import jp.kz2601.metamon.dsl.screen
import jp.kz2601.metamon.renderer.form.FormDraft
import jp.kz2601.metamon.renderer.navigation.NavigationResolver

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
    val DELETE_CONFIRM get() = Strings["desktop_keys_delete_confirm"]
    val NO_NEW get() = Strings["desktop_keys_no_new"]
    val NO_SAVE get() = Strings["desktop_keys_no_save"]
    val PALETTE_PLACEHOLDER get() = Strings["desktop_keys_palette_placeholder"]
    val PALETTE_EMPTY get() = Strings["desktop_keys_palette_empty"]
    val KIND_ACTION get() = Strings["desktop_keys_kind_action"]
    val KIND_SCREEN get() = Strings["desktop_keys_kind_screen"]
    val KIND_APP get() = Strings["desktop_keys_kind_app"]
    val HELP_CLOSE get() = Strings["desktop_close"]
    val HELP_TAB get() = Strings["desktop_keys_help_tab"]
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
        // macOS のキーボードの delete は Backspace。項目の削除は ⌘⌫(Finder と同じ)。fn+delete(前方削除)も受ける
        Shortcut.DELETE -> if (os == HostOs.MAC) listOf(KeyChord(Key.Backspace, primary = true), KeyChord(Key.Delete)) else listOf(KeyChord(Key.Delete))
    }

    fun matches(c: KeyChord, p: KeyPress, os: HostOs): Boolean {
        if (c.key != p.key || p.alt || c.shift != p.shift) return false
        val primary = if (os == HostOs.MAC) p.meta else p.ctrl
        val other = if (os == HostOs.MAC) p.ctrl else p.meta
        return primary == c.primary && !other
    }

    /** 押したキーのショートカット。印刷も返す(処理は Print.kt)。 */
    fun match(p: KeyPress, os: HostOs): Shortcut? = Shortcut.entries.firstOrNull { s -> chords(s, os).any { matches(it, p, os) } }

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

    private fun keyName(k: Key, os: HostOs): String = when (k) {
        Key.Escape -> "Esc"
        Key.Delete -> if (os == HostOs.MAC) "⌦" else "Delete"
        Key.Backspace -> if (os == HostOs.MAC) "⌫" else "Backspace"
        Key.F1 -> "F1"
        Key.Slash -> "/"
        Key.K -> "K"
        Key.N -> "N"
        Key.P -> "P"
        Key.S -> "S"
        Key.F -> "F"
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

/** Ctrl+F の行き先: 今描いている searchBar。後から描いた物(sheet の中)を先に。 */
object SearchTargets {
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

/** searchBar に付ける(Ctrl+F で入る)。 */
fun Modifier.searchShortcutTarget(): Modifier = composed {
    val r = remember { FocusRequester() }
    SearchTargets.Register(r)
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

    var paletteOpen by mutableStateOf(false)
    var helpOpen by mutableStateOf(false)
    val root = FocusRequester()
    val modal = FocusRequester()

    private val topScreen: Screen? get() = dsl.screen(session.top.screen)
    private fun ctx() = session.top.let { ActionContext(it, session.form(it)) }

    fun handle(e: KeyEvent, bubble: Boolean): Boolean {
        if (paletteOpen || helpOpen) return false // パレット・一覧が自分で受ける
        val p = KeyPress.of(e) ?: return false
        val s = Keymap.match(p, os) ?: return false
        if (s == Shortcut.PRINT || s.bubble != bubble) return false // 印刷は Print.kt の Ctrl+P
        return run(s)
    }

    /** ショートカット 1 つ。この画面で使えなければ false(キーは他へ流す)。 */
    fun run(s: Shortcut): Boolean {
        val screen = topScreen ?: return false
        val form = session.form(session.top)
        when (s) {
            Shortcut.PALETTE -> paletteOpen = true
            Shortcut.HELP -> helpOpen = true
            Shortcut.PRINT -> print()
            Shortcut.NEW -> ScreenShortcuts.newAction(dsl, screen)?.let { dispatch(it, ctx()) } ?: run { session.message = KeyboardText.NO_NEW }
            Shortcut.SAVE -> ScreenShortcuts.saveAction(screen, form)?.let { dispatch(it, ctx()) } ?: run { session.message = KeyboardText.NO_SAVE }
            Shortcut.SEARCH -> if (!SearchTargets.focusTop()) paletteOpen = true
            Shortcut.DELETE -> dispatch(ScreenShortcuts.deleteAction(screen, form) ?: return false, ctx())
            Shortcut.CLOSE -> if (NavigationResolver.canClose(session.nav, session.top)) session.back(session.top) else return false
        }
        return true
    }

    /** 画面へ移る(タブはタブを選ぶ・他は push)。 */
    fun openScreen(id: String) {
        if (dsl.app.tabs.any { it.screen == id }) {
            session.selectTab(id)
            return
        }
        dispatch(Action(type = ActionType.NAVIGATE, screen = id, presentation = Presentation.PUSH), ctx())
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
        val apps = openApp?.let { open ->
            BundledApps.all().map { (id, name) -> PaletteCommand("app:$id", name, PaletteCommand.Kind.APP, keywords = listOf(id)) { open(id) } }
        }.orEmpty()
        return shortcuts + buttons + screens + apps
    }

    /** 一番外に付ける: Ctrl の組みは入力欄より先に、Esc・Delete・F1 は入力欄の後に受ける。 */
    val modifier: Modifier = Modifier
        .onPreviewKeyEvent { handle(it, bubble = false) }
        .onKeyEvent { handle(it, bubble = true) }
        .focusRequester(root)
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
        if (paletteOpen || helpOpen) return
        if (modalOpen && runCatching { modal.requestFocus(FocusDirection.Enter) }.getOrDefault(false)) return
        runCatching { root.requestFocus() }
    }

    /** パレット・一覧。DesktopApp の画面の上(Surface の中の最後)に置く。 */
    @Composable
    fun Overlay() {
        val modalOpen = session.nav.sheet != null || session.nav.fullScreen != null
        LaunchedEffect(session.nav) { focusAfterNavigation(modalOpen) }
        if (paletteOpen) {
            CommandPalette(remember(session.nav, session.store) { commands() }, os) {
                paletteOpen = false
                runCatching { root.requestFocus() }
            }
        }
        if (helpOpen) {
            ShortcutHelp(os) {
                helpOpen = false
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
): DesktopKeyboard = remember(dsl, session) { DesktopKeyboard(dsl, session) }.also {
    it.dispatch = dispatch
    it.print = print
    it.openApp = openApp
}
