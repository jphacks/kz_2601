import Foundation
import MetamonDSL

/// aiChat inputEntity の記録選び(AIChatMaterial)に使う、外部ライブラリ無しの簡単なキーワード検索。
/// 語: NFKC + 小文字にして、かな・漢字・ハングルの並びは 2 文字ずつ(1 文字だけの並びはその 1 文字)、
/// 英数字の並びは 2 文字以上の語。点: 発言の語のうち記録に含まれるものの重み log(1 + 件数 / 含む件数) の和
/// (どの記録にもある「ます」「した」は軽い)。語は名前順に足すので、同じ入力なら端末・起動ごとに同じ点になる。
enum AIChatRelevance {
    /// 発言・記録の語。
    static func tokens(_ s: String) -> Set<String> {
        var out = Set<String>()
        var word = String.UnicodeScalarView()
        var cjk: [Unicode.Scalar] = []
        func flush() {
            if word.count >= 2 { out.insert(String(word)) }
            word.removeAll()
            if cjk.count == 1 { out.insert(String(Character(cjk[0]))) }
            if cjk.count >= 2 {
                for i in 0..<(cjk.count - 1) {
                    var v = String.UnicodeScalarView()
                    v.append(cjk[i]); v.append(cjk[i + 1])
                    out.insert(String(v))
                }
            }
            cjk.removeAll()
        }
        for u in s.precomposedStringWithCompatibilityMapping.lowercased().unicodeScalars {
            if isCJK(u) {
                if !word.isEmpty { flush() }
                cjk.append(u)
            } else if u.properties.isAlphabetic || u.properties.numericType != nil {
                if !cjk.isEmpty { flush() }
                word.append(u)
            } else {
                flush()
            }
        }
        flush()
        return out
    }

    static func isCJK(_ u: Unicode.Scalar) -> Bool {
        switch u.value {
        case 0x3005, 0x3007, // 々 〇
             0x3040...0x30FF, // ひらがな・カタカナ(ー を含む)
             0x3400...0x4DBF, 0x4E00...0x9FFF, 0xF900...0xFAFF, // 漢字
             0xAC00...0xD7AF: // ハングル
            true
        default:
            false
        }
    }

    /// 各文書の点(`documents` と同じ順)。0 = 発言の語を 1 つも含まない。
    static func scores(query: Set<String>, documents: [String]) -> [Double] {
        let terms = query.sorted()
        let sets = documents.map { tokens($0).intersection(query) }
        var df: [String: Int] = [:]
        for s in sets { for t in s { df[t, default: 0] += 1 } }
        let n = Double(documents.count)
        return sets.map { s in
            terms.reduce(0.0) { acc, t in s.contains(t) ? acc + log(1 + n / Double(df[t] ?? 1)) : acc }
        }
    }

    /// 1 行に入りきらない記録(取り込んだ PDF など)を `maxBytes` に切る。発言の語が一番多く入る所を
    /// 約 512 バイトの区切りで探し、先頭でなければ「- …」から始める。語が無い・先頭が一番なら頭から(#748 と同じ)。
    static func window(_ line: String, query: Set<String>, maxBytes: Int, chunkBytes: Int = 512) -> String {
        let head = AITextLimits.clip(line, maxBytes: maxBytes)
        guard line.utf8.count > maxBytes, !query.isEmpty else { return head }
        // 文字の区切りで約 chunkBytes ずつに分ける
        var chunks: [Substring] = []
        var start = line.startIndex, used = 0
        for i in line.indices {
            let n = String(line[i]).utf8.count
            if used + n > chunkBytes, i > start {
                chunks.append(line[start..<i])
                start = i
                used = 0
            }
            used += n
        }
        chunks.append(line[start...])
        let hits = chunks.map { tokens(String($0)).intersection(query).count }
        let marker = "- …"
        let budget = maxBytes - marker.utf8.count
        var best = 0, bestScore = -1
        for s in chunks.indices {
            var bytes = 0, score = 0
            for c in s..<chunks.count {
                bytes += chunks[c].utf8.count
                if bytes > budget { break }
                score += hits[c]
            }
            if score > bestScore { best = s; bestScore = score } // 同点は前(先頭寄り)
        }
        guard best > 0, bestScore > 0 else { return head }
        return AITextLimits.clip(marker + String(line[chunks[best].startIndex...]), maxBytes: maxBytes)
    }
}
