import Foundation

/// 「これでいく」の前に出す有料機能の確認カードの中身(#77)。
public struct PaidConfirmation: Equatable, Sendable {
    public var features: [PaidFeature]
    /// 「購入して作る」で generate に渡す文(ready の transcript / 案の instruction)
    public var paidText: String
    /// 「この機能なしで作る」で generate に渡す文(backend の freeTranscript / freeInstruction)
    public var freeText: String

    public init(features: [PaidFeature], paidText: String, freeText: String) {
        self.features = features
        self.paidText = paidText
        self.freeText = freeText
    }

    /// カードの見出しに出す機能名(「みんなで共有するデータ」など。複数は「・」でつなぐ)
    public var featureNames: String {
        let names = features.map(\.title).filter { !$0.isEmpty }
        return names.isEmpty ? "この機能" : names.joined(separator: "・")
    }
}

/// 有料機能の確認カードを出すかの判定(#77)。純粋関数のみ。判定そのもの(何が有料か)は backend の chat 応答が持つ。
/// - Pro 会員なら出さない / 回数制限と同じく無効化中(Debug の既定)なら出さない(`ProGate.isEnforced`)
/// - 旧 backend(requiresPro を送らない)なら出さない
public enum PaidFeatureCheck {
    /// backend が freeText を返さなかった時の予備(有料機能を抜く指示)
    public static let fallbackFreeSuffix = "(無料版で作る: 他のユーザーとデータを共有する機能などの有料機能は入れない)"

    public static func confirmation(requiresPro: Bool?, features: [PaidFeature]?, paidText: String, freeText: String?,
                                    isPro: Bool, enforced: Bool) -> PaidConfirmation? {
        guard requiresPro == true, enforced, !isPro else { return nil }
        let paid = paidText.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !paid.isEmpty else { return nil }
        let free = freeText?.trimmingCharacters(in: .whitespacesAndNewlines)
        return PaidConfirmation(features: features ?? [], paidText: paid,
                                freeText: free?.isEmpty == false ? free! : paid + "\n" + fallbackFreeSuffix)
    }

    /// ready の「これでいく」。`text` が ready の transcript の時だけ(予備の依頼文で作る時は判定材料が無いので出さない)。
    public static func confirmation(for turn: ChatTurn?, text: String, isPro: Bool, enforced: Bool) -> PaidConfirmation? {
        guard let turn, turn.kind == .ready,
              turn.transcript?.trimmingCharacters(in: .whitespacesAndNewlines) == text.trimmingCharacters(in: .whitespacesAndNewlines) else { return nil }
        return confirmation(requiresPro: turn.requiresPro, features: turn.paidFeatures, paidText: text,
                            freeText: turn.freeTranscript, isPro: isPro, enforced: enforced)
    }

    /// 案のカードの「これでいく」。
    public static func confirmation(for option: ChatOption, isPro: Bool, enforced: Bool) -> PaidConfirmation? {
        confirmation(requiresPro: option.requiresPro, features: option.paidFeatures, paidText: option.instruction,
                     freeText: option.freeInstruction, isPro: isPro, enforced: enforced)
    }
}
