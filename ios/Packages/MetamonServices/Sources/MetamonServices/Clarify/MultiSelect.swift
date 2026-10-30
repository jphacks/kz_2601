import Foundation

/// #220: 複数選択(proposal の案 / question のチップ)。
public enum MultiSelect {
    /// 選んだ案の instruction を 1 回の generate 用につなぐ。1 件ならそのまま、複数なら「・」の箇条書き(改行区切り)。
    public static func joinedInstruction(_ instructions: [String]) -> String {
        let items = instructions.map { $0.trimmingCharacters(in: .whitespacesAndNewlines) }.filter { !$0.isEmpty }
        if items.count <= 1 { return items.first ?? "" }
        return items.map { "・" + $0 }.joined(separator: "\n")
    }

    /// 選んだチップを 1 つの回答にする(「、」区切り)。
    public static func joinedAnswer(_ chips: [String]) -> String {
        chips.map { $0.trimmingCharacters(in: .whitespacesAndNewlines) }.filter { !$0.isEmpty }.joined(separator: "、")
    }

    /// 「これでいく(N 件)」「決定」を押せるか(1 件以上)。
    public static func canSubmit(selectedCount: Int) -> Bool { selectedCount > 0 }

    /// 選んだ案を 1 つの案にまとめる(並び順は渡された順)。有料機能は和集合、無料版は各案の無料版(なければ元の指示)をつなぐ。
    public static func combined(_ options: [ChatOption]) -> ChatOption? {
        guard let first = options.first else { return nil }
        if options.count == 1 { return first }
        let pro = options.contains { $0.requiresPro == true }
        var features: [PaidFeature] = []
        for feature in options.flatMap({ $0.paidFeatures ?? [] }) where !features.contains(where: { $0.id == feature.id }) {
            features.append(feature)
        }
        let free = options.map { $0.requiresPro == true ? ($0.freeInstruction ?? "") : $0.instruction }
        return ChatOption(
            id: options.map(\.id).joined(separator: "+"),
            title: options.map(\.title).joined(separator: "、"),
            detail: "",
            instruction: joinedInstruction(options.map(\.instruction)),
            requiresPro: pro ? true : nil,
            paidFeatures: pro ? features : nil,
            freeInstruction: pro ? joinedInstruction(free) : nil)
    }
}
