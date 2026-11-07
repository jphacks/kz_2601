import Foundation

/// `text.highlight`: 文中の語に印をつける。words(決まった語)か field(screen.entity の field。値を「,」「、」改行で区切る)。
/// 照合は大文字小文字・全角半角を区別せず(NFKC)、日本語は語の一部でも合う。語は `maxWords` 個まで。
public struct TextHighlight: Decodable, Sendable, Equatable {
    public var words: [String]?
    public var field: String?
    public var style: String?   // marker(既定)| bold

    public static let maxWords = 20
    public static let maxWordLength = 50

    public init(words: [String]? = nil, field: String? = nil, style: String? = nil) {
        self.words = words
        self.field = field
        self.style = style
    }

    /// 印をつける語(空・重複を除き、先頭から `maxWords` 個)。`fieldValue` は field の今の値。
    public func resolvedWords(fieldValue: String?) -> [String] {
        let raw: [String]
        if let words, !words.isEmpty {
            raw = words
        } else if field != nil, let fieldValue {
            raw = fieldValue.precomposedStringWithCompatibilityMapping
                .split(whereSeparator: { $0 == "," || $0 == "、" || $0.isNewline }).map(String.init)
        } else {
            raw = []
        }
        var seen = Set<String>()
        var out: [String] = []
        for w in raw {
            let t = w.trimmingCharacters(in: .whitespaces)
            guard !t.isEmpty, t.count <= Self.maxWordLength, seen.insert(TextHighlightMatcher.fold(t)).inserted else { continue }
            out.append(t)
            if out.count == Self.maxWords { break }
        }
        return out
    }
}

/// 語の照合(描画から切り離して単体テストする)。
public enum TextHighlightMatcher {
    /// 照合用の形: NFKC + 小文字。
    public static func fold(_ s: String) -> String { s.precomposedStringWithCompatibilityMapping.lowercased() }

    /// `text` の中で `words` に合う範囲(重なりはまとめる・文字の途中では切らない)。
    public static func ranges(in text: String, words: [String]) -> [Range<String.Index>] {
        guard !text.isEmpty, !words.isEmpty else { return [] }
        // 元の文字ごとに畳んだ scalar を並べ、scalar → 元の文字の位置を覚える。
        var hay: [Unicode.Scalar] = []
        var owner: [Int] = []
        var starts: [String.Index] = []
        for idx in text.indices {
            starts.append(idx)
            for u in fold(String(text[idx])).unicodeScalars {
                hay.append(u)
                owner.append(starts.count - 1)
            }
        }
        var spans: [(Int, Int)] = [] // 元の文字の [lo, hi)
        for w in words {
            let needle = Array(fold(w).unicodeScalars)
            guard !needle.isEmpty, needle.count <= hay.count else { continue }
            var i = 0
            while i + needle.count <= hay.count {
                if hay[i] == needle[0], Array(hay[i..<i + needle.count]) == needle {
                    spans.append((owner[i], owner[i + needle.count - 1] + 1))
                    i += needle.count
                } else {
                    i += 1
                }
            }
        }
        spans.sort { $0.0 < $1.0 }
        var merged: [(Int, Int)] = []
        for s in spans {
            if let last = merged.last, s.0 <= last.1 {
                merged[merged.count - 1].1 = max(last.1, s.1)
            } else {
                merged.append(s)
            }
        }
        return merged.map { lo, hi in starts[lo]..<(hi < starts.count ? starts[hi] : text.endIndex) }
    }
}
