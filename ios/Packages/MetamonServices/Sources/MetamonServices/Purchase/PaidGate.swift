import Foundation
import MetamonDSL

/// サーバーに負荷のかかる有料機能を、課金画面(PaywallSheet)を通さずに使わせないための判定(#414)。純粋関数のみ。
/// - 生成の前: チャットの ready / 案の `requiresPro`(`PaidFeatureCheck`)
/// - 生成の後: チャットを通らない経路(即生成・予備文・圏外の保留・元に戻す / やり直す)は、できた DSL の entity.scope で見る
/// シェアの参加・公開アプリのインストールは対象外(参加者は無料・作成者の Pro で持つ。正典「課金・Pro」2026-11-06・H5)。
/// 何が有料かの正典は backend `paid_features.json`。DSL から見えるのは sharedData の `dslEntityScopes`(shared / public)だけ。
public enum PaidGate {
    /// 課金画面を出すか(Pro なら出さない・課金の制限が無効のデモ端末でも出さない)。
    public static func needsPaywall(isPro: Bool, enforced: Bool) -> Bool { enforced && !isPro }

    /// 自分以外も見られる(サーバーで全員に同期する)記録を使うか。
    public static func usesSharedData(_ dsl: AppDSL) -> Bool {
        dsl.entities.contains { dsl.scope(of: $0.id).isVisibleToOthers }
    }

    /// この変化で共有の記録が新しく入ったか。もとから共有のアプリの修正は聞き直さない(backend `newPaid` と同じ)。
    public static func introducesSharedData(from before: AppDSL?, to after: AppDSL) -> Bool {
        usesSharedData(after) && !(before.map(usesSharedData) ?? false)
    }

    /// 生成の後の判定。新しく共有の記録が入り、Pro でなければ課金画面。
    public static func generatedNeedsPaywall(before: AppDSL?, after: AppDSL, isPro: Bool, enforced: Bool) -> Bool {
        needsPaywall(isPro: isPro, enforced: enforced) && introducesSharedData(from: before, to: after)
    }

    /// 課金画面を閉じた(買わなかった)時に、生成でできた共有の記録を外す修正の指示。
    /// backend `paid_features.json` の sharedData.withoutInstruction と同じ意味(entity の scope を省略 = private)。
    public static func dropSharedDataInstruction(english: Bool) -> String {
        english
            ? "Make every shared or public record private to this user only: remove the scope of every entity. Do not include any feature that shares data with other users."
            : "共有・公開している記録をすべて自分だけの記録にして。entity の scope はすべて省略して private にし、他のユーザーとデータを共有する機能は入れない"
    }
}
