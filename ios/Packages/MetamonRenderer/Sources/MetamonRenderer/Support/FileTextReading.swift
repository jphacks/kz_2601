import PDFKit
#if canImport(UIKit)
import UIKit
#else
import AppKit
#endif
import UniformTypeIdentifiers
import MetamonDSL

/// `fileImport` が読むファイルの種類と、テキスト / PDF の本文の取り出し(録音は `AudioFileImport`)。すべて端末内で、ファイルはコピーも保存もしない。
enum FileTextReading {
    enum Kind: Equatable {
        case csv, text, pdf, audio
    }

    /// `accept` → ファイル選択に出す種類。
    static func contentTypes(_ accept: [String]) -> [UTType] {
        var out: [UTType] = []
        for a in accept {
            switch a {
            case "csv": out.append(.commaSeparatedText)
            case "text": out.append(.plainText)
            case "pdf": out.append(.pdf)
            case "audio": out.append(.audio)
            default: break
            }
        }
        return out
    }

    /// 選ばれたファイルの種類(`accept` に無いものは nil)。CSV は plainText にも当たるので先に見る。
    static func kind(of url: URL, accept: [String]) -> Kind? {
        let type = UTType(filenameExtension: url.pathExtension.lowercased()) ?? .data
        if accept.contains("csv"), type.conforms(to: .commaSeparatedText) { return .csv }
        if accept.contains("pdf"), type.conforms(to: .pdf) { return .pdf }
        if accept.contains("text"), type.conforms(to: .plainText) { return .text }
        if accept.contains("audio"), type.conforms(to: .audio) { return .audio }
        return nil
    }

    /// PDF の本文。文字の層があるページはそのまま、無いページ(スキャン)は画像にして端末内の文字認識(最大 `maxOCRPages` ページ)。
    static func pdfText(_ data: Data) -> String? {
        guard let doc = PDFDocument(data: data), !doc.isLocked else { return nil }
        var pages: [String] = []
        var ocr = 0
        for i in 0..<doc.pageCount {
            guard let page = doc.page(at: i) else { continue }
            let text = page.string?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
            if !text.isEmpty {
                pages.append(text)
            } else if ocr < RecordImport.maxOCRPages, let image = render(page) {
                ocr += 1
                let lines = PhotoVision.textLines(in: image)
                if !lines.isEmpty { pages.append(lines.joined(separator: "\n")) }
            }
        }
        return pages.joined(separator: "\n\n")
    }

    /// ページを文字認識できる大きさ(長辺 2000px まで)の画像に。
    private static func render(_ page: PDFPage) -> CGImage? {
        let box = page.bounds(for: .mediaBox)
        guard box.width > 0, box.height > 0 else { return nil }
        let scale = min(2000 / max(box.width, box.height), 3)
        let size = CGSize(width: box.width * scale, height: box.height * scale)
        return page.thumbnail(of: size, for: .mediaBox).cgImage
    }
}
