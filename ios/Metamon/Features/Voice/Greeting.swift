import Foundation

/// 音声画面の問いかけ文言(#42)。見出し・サブ文言はここにだけ置き、多言語化(#44)ではこのファイルを差し替える。
/// 時間帯の挨拶(おはよう/こんにちは/こんばんは)を前置きし、初回・再訪・新規作成・既存アプリの改変で出し分ける。
enum Greeting {
    enum Context: Equatable {
        /// 初回起動(まだ何も作っていない)
        case firstVisit
        /// 2 回目以降の起動で初期画面
        case returning
        /// 変身一覧の「+ 新しいアプリ」/ 発話「別のアプリ」
        case newApp
        /// 変身一覧の「Tailor It に戻る」/ アプリのリセット
        case home
        /// 既にアプリがある(FAB からの音声シート)
        case editing
    }

    struct Copy: Equatable {
        var title: String
        var subtitle: String
    }

    // MARK: 文言

    static func copy(for context: Context, date: Date = .now, calendar: Calendar = .current) -> Copy {
        let hello = timeOfDay(date, calendar: calendar)
        switch context {
        case .firstVisit:
            return Copy(title: "どんなアプリが欲しいですか?",
                        subtitle: "\(hello)。話しかけるだけで、アプリが生まれ変わります")
        case .returning:
            return Copy(title: "おかえりなさい。\n今日は何を作りますか?",
                        subtitle: "\(hello)。「アプリ一覧」と話すか、マイクを長押しすると前に作ったアプリを呼び出せます")
        case .newApp:
            return Copy(title: "新しいアプリですね。\nどんなアプリにしますか?",
                        subtitle: "前のアプリは、マイクの長押しからいつでも戻せます")
        case .home:
            return Copy(title: "おかえりなさい。\n次は何を作りますか?",
                        subtitle: "\(hello)。前のアプリは、マイクの長押しからいつでも戻せます")
        case .editing:
            return Copy(title: "このアプリをどう変えますか?",
                        subtitle: "話しかけると、このアプリが育ちます")
        }
    }

    /// 変身一覧シートの見出し。
    static let switcherTitle = "変身一覧"
    static let switcherSubtitle = "タップで切り替え。マイクを長押しすると、いつでもここに戻れます"
    static let switcherEmpty = "まだ他のアプリはありません。\n「+ 新しいアプリ」から 2 つ目を作れます"
    static let currentBadge = "使用中"
    static let newAppAction = "新しいアプリ"
    static let homeAction = "Tailor It に戻る(初期画面)"

    /// 例文チップ。
    static let createExamples = ["家計簿アプリが欲しい", "筋トレの記録", "散歩の写真メモ"]
    static let editExamples = ["ダークモードにして", "グラフを追加して", "メモ欄を増やして"]
    static let examplesLabel = "たとえば"
    static let editExamplesLabel = "たとえば、こんな変更"

    /// 5〜10 時 おはよう / 11〜17 時 こんにちは / それ以外 こんばんは
    static func timeOfDay(_ date: Date = .now, calendar: Calendar = .current) -> String {
        switch calendar.component(.hour, from: date) {
        case 5..<11: return "おはようございます"
        case 11..<18: return "こんにちは"
        default: return "こんばんは"
        }
    }

    // MARK: 初回 / 再訪の判定

    private static let visitsKey = "metamon.visitCount"

    /// このプロセスの起動が再訪か。最初に参照した時点で判定して訪問回数を 1 増やす(起動中は値が変わらない)。
    static let launchIsReturning: Bool = {
        let defaults = UserDefaults.standard
        let count = defaults.integer(forKey: visitsKey)
        defaults.set(count + 1, forKey: visitsKey)
        return count > 0
    }()

    /// 初期画面でどの挨拶を出すか(`AppRuntime.homeEntry` と起動回数から)。
    static func context(homeEntry: AppRuntime.HomeEntry, isInitial: Bool) -> Context {
        guard isInitial else { return .editing }
        switch homeEntry {
        case .launch: return launchIsReturning ? .returning : .firstVisit
        case .newApp: return .newApp
        case .home: return .home
        }
    }
}
