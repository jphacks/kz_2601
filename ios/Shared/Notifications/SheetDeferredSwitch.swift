/// 押された通知でアプリを切り替える時、シェルのシート(会話・頼み方の例)が開いていれば閉じるまで待つ(#943)。
/// 待たないと、入力を捨てる確認がシートの裏に出て見えない(⌥⌘↑↓ も同じ理由でシートの間は止めている)。
/// 待つのは最後に押された 1 件だけ。シートを閉じたら、その時に確認を通して切り替える。
struct SheetDeferredSwitch: Equatable {
    private(set) var pending: String?

    /// 通知が押された。シートが無ければ今切り替える先を返す。あれば覚えて nil
    mutating func request(_ appId: String, sheetOpen: Bool) -> String? {
        guard sheetOpen else {
            pending = nil
            return appId
        }
        pending = appId
        return nil
    }

    /// シートを閉じた。待っていた切り替え先を返して忘れる(まだ別のシートが開いていれば待ち続ける)
    mutating func sheetClosed(sheetOpen: Bool = false) -> String? {
        guard !sheetOpen, let appId = pending else { return nil }
        pending = nil
        return appId
    }
}
