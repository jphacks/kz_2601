package jp.kz2601.metamon.desktop

/** ランチャーの右クリック(.desktop の Actions)から来る操作(#2992)。iOS のホーム画面アイコンの長押しメニューに当たる。 */
enum class LaunchAction(val flag: String, val desktopId: String, val stringKey: String, val exec: String) {
    QUICK_RECORD("--quick-record", "QuickRecord", "desktop_launch_action_quick_record", "tailor-it --quick-record"),
    NEW_WINDOW("--new-window", "NewWindow", "desktop_launch_action_new_window", "tailor-it --new-window"),
    TALK("--talk", "Talk", "desktop_launch_action_talk", "tailor-it --talk"),
}

/** 起動の引数を「操作」と「位置の引数(アプリ・記録の JSON・最初の画面)」に分けた物。 */
data class ParsedLaunchArgs(val action: LaunchAction?, val positional: List<String>)

object LaunchArgs {
    /**
     * `--quick-record` などの旗を取り除き、残りを位置の引数として返す。
     * 旗が複数あれば最初の物を使う。知らない `--` の旗は位置の引数に混ぜず捨てる(アプリ名と取り違えない)。
     * `--` 以降は全部位置の引数。
     */
    fun parse(args: List<String>): ParsedLaunchArgs {
        var action: LaunchAction? = null
        val positional = mutableListOf<String>()
        var rest = false
        for (a in args) {
            when {
                rest -> positional += a
                a == "--" -> rest = true
                a.startsWith("--") -> if (action == null) action = LaunchAction.entries.firstOrNull { it.flag == a }
                else -> positional += a
            }
        }
        return ParsedLaunchArgs(action, positional)
    }

    /**
     * .desktop の `[Desktop Action ...]` の節。[label] は操作ごとの表示名(日本語・英語は呼ぶ側で引く)。
     * 先頭の `Actions=` の行は [actionsLine]。
     */
    fun desktopActionSections(label: (LaunchAction) -> String, execName: String = "tailor-it"): String =
        LaunchAction.entries.joinToString("\n") { a ->
            "[Desktop Action ${a.desktopId}]\nName=${label(a)}\nExec=${a.exec.replaceFirst("tailor-it", execName)}\n"
        }

    /** `Actions=QuickRecord;NewWindow;Talk;`(末尾の `;` も含む)。 */
    fun actionsLine(): String = "Actions=" + LaunchAction.entries.joinToString("") { it.desktopId + ";" }
}
