import Foundation
import MetamonServices

/// 設定の「サインインの方法」(#361)の出し分け。iOS の `LinkedAccountsSection` と Mac の `MacLinkedAccountsSection` で共有する。
enum LinkedAccountsRules {
    /// 欄を出すか。匿名のユーザー・未ログインには出さない(匿名 → Apple の引き継ぎはアカウントの欄・#276)
    static func showsSection(for user: AuthUser?) -> Bool {
        guard let user else { return false }
        return !user.isAnonymous
    }

    /// つながっている方法だけ(つなげない Google の「つながっていません」は出さない)
    static func rows(for user: AuthUser) -> [SignInProvider] {
        SignInProvider.allCases.filter(user.providers.contains)
    }

    /// 「Apple で続ける」は Apple がまだつながっていない時だけ
    static func offersApple(to user: AuthUser) -> Bool { !user.providers.contains(.apple) }
}
