import Foundation
import MetamonDSL

extension MockSeed {
    /// `examples/chat.json`(#260)の会話。相手(さくら・たけし)と自分(`MockAuthService.fixedUID`)が数分おきに送った 5 通。
    static func chatSeed(now: Date) -> [Record] {
        let iso = ISO8601DateFormatter()
        let lines: [(author: String, name: String, text: String)] = [
            ("dummy-sakura", "さくら", "今週の練習、土曜の 10 時からで大丈夫？"),
            ("dummy-sakura", "さくら", "体育館は第 2 を取ってあります"),
            (MockAuthService.fixedUID, "自分", "大丈夫です！ボール持っていきます"),
            ("dummy-takeshi", "たけし", "少し遅れるかも。11 時には着きます"),
            ("dummy-sakura", "さくら", "了解、先に始めてるね"),
        ]
        return lines.enumerated().map { i, line in
            let at = iso.string(from: now.addingTimeInterval(Double(i - lines.count) * 240))
            return Record(id: "msg\(i + 1)", data: ["text": .string(line.text)], createdAt: at, updatedAt: at,
                          author: line.author, authorName: line.name)
        }
    }
}
