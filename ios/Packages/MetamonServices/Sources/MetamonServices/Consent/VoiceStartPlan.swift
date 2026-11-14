import Foundation

/// 声で頼む時、録音の前に音声モデルのダウンロードを聞くか(Mac・#966)。
/// iOS の「マイクを押した時に確認する」(M14)と同じ考えで、Mac では次を足す:
/// - 遅い Mac(Intel・#363)では、Apple の端末内の認識が使えるなら聞かずにそれで録音する(モデルは本人が選んだ時だけ)
/// - 「あとで」を選んだ後は、この起動の間は聞き直さない(仮の聞き取りで録音する・使えなければ文字で頼んでもらう)
public enum VoiceStartPlan: Equatable, Sendable {
    /// そのまま録音する(本命が準備済み・準備中・または仮の聞き取りを使う)
    case record
    /// 録音の前にサイズを示してダウンロードを聞く
    case askDownload
    /// 声は使えない。文字で頼むよう案内する(ダウンロードのボタンは出す)
    case textOnly

    /// - Parameters:
    ///   - needsDownload: 端末にモデルが無く、まだダウンロードを始めていない
    ///   - isApproved: 前に「ダウンロード」を選んだ
    ///   - isExpensiveNetwork: 従量回線(承認済みでも聞き直す・#275)
    ///   - slowHardware: 認識が遅い見込みの Mac(Intel)
    ///   - provisionalAvailable: Apple の端末内の認識が使える
    ///   - askedThisLaunch: この起動の間に一度聞いた
    public static func decide(needsDownload: Bool, isApproved: Bool, isExpensiveNetwork: Bool,
                              slowHardware: Bool, provisionalAvailable: Bool, askedThisLaunch: Bool) -> VoiceStartPlan {
        guard needsDownload else { return .record }
        if isApproved && !isExpensiveNetwork { return .record }
        if slowHardware && provisionalAvailable { return .record }
        if !askedThisLaunch { return .askDownload }
        return provisionalAvailable ? .record : .textOnly
    }
}
