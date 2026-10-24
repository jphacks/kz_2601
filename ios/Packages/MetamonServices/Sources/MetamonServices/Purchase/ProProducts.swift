import Foundation

/// 「Tailor It Pro」の商品定義(#77)。本番の商品 ID に差し替える時はここだけ直す。
/// ローカル検証は `ios/StoreKit/TailorItPro.storekit`(Scheme の StoreKit Configuration)が同じ ID を持つ。
public enum ProProducts {
    /// 月額サブスク(自動更新)。App Store Connect に同じ ID で登録する。
    public static let monthly = "jp.kz2601.metamon.pro.monthly"
    /// サブスクリプショングループ名(App Store Connect 側の参照名と合わせる)。
    public static let subscriptionGroup = "Tailor It Pro"
    public static let all: [String] = [monthly]
    /// 無料で 1 日に新規作成できるアプリの数。
    public static let dailyFreeCreations = 3
}
