import Foundation

/// 共有アプリへの投稿に出す表示名(#250)。本人が決めた名前だけを使う。
/// Apple の氏名・メールアドレス・端末名は、本人が公開すると決めていないので使わない。未設定なら送らず、backend が「匿名」を付ける。
/// アカウント削除(`wipeLocalData`)で UserDefaults ごと消える。
enum AuthorProfile {
    static let nameKey = "authorProfile.name"
    static let askedKey = "authorProfile.asked"
    /// backend の上限(64 文字)より短く。一覧の 1 行に収まる長さ
    static let maxLength = 20

    /// 投稿に付ける名前。空白だけ・未設定は nil。
    static var name: String? {
        get { normalized(UserDefaults.standard.string(forKey: nameKey) ?? "") }
        set { UserDefaults.standard.set(normalized(newValue ?? "") ?? "", forKey: nameKey) }
    }

    /// 初めての共有投稿で一度だけ名前を聞いたか(設定で名前を決めた時も済みにする)。
    static var asked: Bool {
        get { UserDefaults.standard.bool(forKey: askedKey) }
        set { UserDefaults.standard.set(newValue, forKey: askedKey) }
    }

    static func normalized(_ raw: String) -> String? {
        let s = String(raw.trimmingCharacters(in: .whitespacesAndNewlines).prefix(maxLength))
        return s.isEmpty ? nil : s
    }
}
