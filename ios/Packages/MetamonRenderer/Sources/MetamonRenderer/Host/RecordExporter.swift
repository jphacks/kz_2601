import SwiftUI
import UIKit
import MetamonDSL

/// 書き出したファイル(#421)。ホストが共有シートに渡す。
public struct ExportFile: Identifiable, Equatable, Sendable {
    public var url: URL
    public var id: URL { url }
    public init(url: URL) { self.url = url }
}

public enum ExportError: Error, Equatable {
    case unknownEntity
    case unknownFormat
}

/// `export` アクションの中身。表は list / table と同じ経路(computed 合成・filter・`display` の書式)で作る。
@MainActor
public extension DSLRuntime {
    /// 書き出す表。filter は hidden を落とす前に当て、hidden の値は自分が書いた行だけ出す(#262)。
    func exportTable(_ action: Action, now: Date = Date()) throws -> ExportTable {
        guard let id = action.entity, let entity = dsl.entity(id) else { throw ExportError.unknownEntity }
        let records = RecordExport.chronological(unredactedRecords(entity: id, filters: action.filters), entity: entity)
        let title = action.title?.trimmingCharacters(in: .whitespaces).nonEmpty ?? entity.name
        var notes = [L("書き出し日 \(Self.exportDateText(now))")]
        if !action.filters.isEmpty {
            notes.append(L("絞り込み: \(action.filters.map { filterText($0, entity: entity) }.joined(separator: L("・")))"))
        }
        notes.append(L("\(records.count) 件"))
        return RecordExport.table(title: title, entity: entity, records: records, fields: action.fields,
                                  currentUid: currentUserId, notes: notes) { display($0, field: $1) }
    }

    /// 画面にある同じ entity の chart を画像に(PDF の表の上に置く)。
    func exportChartImages(screen: String?, entity: String, width: CGFloat = ExportPDF.contentWidth) -> [UIImage] {
        guard let screen, let s = dsl.screen(screen) else { return [] }
        return s.allComponents.filter { $0.type == .chart && $0.entity == entity }.compactMap { c in
            let view = ChartLeaf(c: c)
                .frame(width: width, height: 240)
                .padding(4)
                .environment(\.dslRuntime, self)
                .environment(\.colorScheme, .light)
            let renderer = ImageRenderer(content: view)
            renderer.scale = 2
            return renderer.uiImage
        }
    }

    /// CSV / PDF を一時フォルダに書き、共有シートに渡すファイルを返す。
    func makeExportFile(_ action: Action, screen: String?, now: Date = Date()) throws -> ExportFile {
        let table = try exportTable(action, now: now)
        let data: Data
        let ext: String
        switch action.format {
        case "csv":
            data = CSVWriter.data(table)
            ext = "csv"
        case "pdf":
            data = ExportPDF.data(table, charts: exportChartImages(screen: screen, entity: action.entity ?? ""))
            ext = "pdf"
        default:
            throw ExportError.unknownFormat
        }
        let dir = FileManager.default.temporaryDirectory
            .appending(path: "export", directoryHint: .isDirectory)
            .appending(path: UUID().uuidString, directoryHint: .isDirectory)
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        let url = dir.appending(path: RecordExport.fileName(table.title, ext: ext))
        try data.write(to: url, options: .atomic)
        return ExportFile(url: url)
    }

    /// 「日付: 今月」「店 = ラーメン太郎」のような絞り込みの説明。
    private func filterText(_ f: Filter, entity: Entity) -> String {
        let field = entity.field(f.field)
        let name = field?.name ?? f.field
        let value = f.valueExpr == nil ? display(f.value ?? .null, field: field) : L("計算した値")
        switch f.op {
        case "today": return L("\(name): 今日")
        case "thisWeek": return L("\(name): 今週")
        case "thisMonth": return L("\(name): 今月")
        case "thisYear": return L("\(name): 今年")
        case "contains":
            let one = f.value?.stringValue.flatMap { refTitle($0, entity: field?.refEntity) } ?? value
            return L("\(name) に \(one) を含む")
        default:
            let symbol = ["eq": "=", "neq": "≠", "gt": ">", "gte": "≥", "lt": "<", "lte": "≤"][f.op] ?? f.op
            return "\(name) \(symbol) \(value)"
        }
    }

    private static func exportDateText(_ date: Date) -> String {
        let f = DateFormatter()
        f.locale = LanguagePreference.shared.locale
        f.dateStyle = .medium
        f.timeStyle = .short
        return f.string(from: date)
    }
}

private extension String {
    var nonEmpty: String? { isEmpty ? nil : self }
}

/// A4 縦の簡単な報告: 見出し・補足・グラフ・表(見出し行はページごとに繰り返す)。
public enum ExportPDF {
    static let page = CGRect(x: 0, y: 0, width: 595.2, height: 841.8)
    static let margin: CGFloat = 40
    public static let contentWidth: CGFloat = 595.2 - 80
    static let rowHeight: CGFloat = 20
    static let cellPad: CGFloat = 4

    public static func data(_ table: ExportTable, charts: [UIImage] = []) -> Data {
        let renderer = UIGraphicsPDFRenderer(bounds: page, format: format(table.title))
        return renderer.pdfData { ctx in
            var y = margin
            var pageNo = 0
            func newPage() {
                ctx.beginPage()
                pageNo += 1
                y = margin
                draw("\(pageNo)", in: CGRect(x: margin, y: page.height - margin + 12, width: contentWidth, height: 14),
                     font: .systemFont(ofSize: 9), color: .secondaryLabel, align: .center)
            }
            newPage()

            y += draw(table.title, in: CGRect(x: margin, y: y, width: contentWidth, height: 60),
                      font: .boldSystemFont(ofSize: 20), color: .label, wraps: true) + 6
            for note in table.notes {
                y += draw(note, in: CGRect(x: margin, y: y, width: contentWidth, height: 40),
                          font: .systemFont(ofSize: 10), color: .secondaryLabel, wraps: true) + 2
            }
            y += 10

            for img in charts {
                let h = min(260, contentWidth * img.size.height / max(img.size.width, 1))
                if y + h > page.height - margin { newPage() }
                img.draw(in: CGRect(x: margin, y: y, width: contentWidth, height: h))
                y += h + 12
            }

            let widths = columnWidths(table)
            func header() {
                UIColor.secondarySystemBackground.resolvedColor(with: .init(userInterfaceStyle: .light)).setFill()
                UIRectFill(CGRect(x: margin, y: y, width: contentWidth, height: rowHeight))
                drawRow(table.headers, y: y, widths: widths, numeric: table.numericColumns, bold: true)
                y += rowHeight
            }
            if table.rows.isEmpty {
                draw(L("記録がありません"), in: CGRect(x: margin, y: y, width: contentWidth, height: 20),
                     font: .systemFont(ofSize: 11), color: .secondaryLabel)
                return
            }
            if y + rowHeight * 2 > page.height - margin { newPage() }
            header()
            for row in table.rows {
                if y + rowHeight > page.height - margin {
                    newPage()
                    header()
                }
                drawRow(row, y: y, widths: widths, numeric: table.numericColumns, bold: false)
                y += rowHeight
                UIColor.separator.resolvedColor(with: .init(userInterfaceStyle: .light)).setFill()
                UIRectFill(CGRect(x: margin, y: y - 0.5, width: contentWidth, height: 0.5))
            }
        }
    }

    /// 内容の長さに応じた列幅(見出しと先頭 50 行で測る)。合計を本文幅に合わせ、細すぎる列は 36pt を下限に。
    static func columnWidths(_ table: ExportTable) -> [CGFloat] {
        guard !table.headers.isEmpty else { return [] }
        let font = UIFont.systemFont(ofSize: 10)
        let natural = table.headers.indices.map { i -> CGFloat in
            let cells = [table.headers[i]] + table.rows.prefix(50).map { i < $0.count ? $0[i] : "" }
            let w = cells.map { ($0 as NSString).size(withAttributes: [.font: font]).width }.max() ?? 0
            return min(max(w + cellPad * 2, 36), 220)
        }
        let total = natural.reduce(0, +)
        return natural.map { $0 * contentWidth / total }
    }

    private static func drawRow(_ cells: [String], y: CGFloat, widths: [CGFloat], numeric: [Bool], bold: Bool) {
        var x = margin
        for (i, w) in widths.enumerated() {
            let text = i < cells.count ? cells[i] : ""
            let right = i < numeric.count && numeric[i]
            draw(text, in: CGRect(x: x + cellPad, y: y + 4, width: w - cellPad * 2, height: rowHeight - 4),
                 font: bold ? .boldSystemFont(ofSize: 10) : .systemFont(ofSize: 10), color: .label,
                 align: right ? .right : .left)
            x += w
        }
    }

    /// 描いた高さを返す。wraps でなければ 1 行で末尾を「…」に。
    @discardableResult
    private static func draw(_ text: String, in rect: CGRect, font: UIFont, color: UIColor,
                             align: NSTextAlignment = .left, wraps: Bool = false) -> CGFloat {
        let style = NSMutableParagraphStyle()
        style.alignment = align
        style.lineBreakMode = wraps ? .byWordWrapping : .byTruncatingTail
        let attrs: [NSAttributedString.Key: Any] = [
            .font: font, .paragraphStyle: style,
            .foregroundColor: color.resolvedColor(with: .init(userInterfaceStyle: .light)),
        ]
        let s = text as NSString
        let options: NSStringDrawingOptions = wraps ? [.usesLineFragmentOrigin] : [.usesLineFragmentOrigin, .truncatesLastVisibleLine]
        let box = wraps ? rect : CGRect(x: rect.minX, y: rect.minY, width: rect.width, height: font.lineHeight)
        s.draw(with: box, options: options, attributes: attrs, context: nil)
        return ceil(s.boundingRect(with: box.size, options: options, attributes: attrs, context: nil).height)
    }

    private static func format(_ title: String) -> UIGraphicsPDFRendererFormat {
        let f = UIGraphicsPDFRendererFormat()
        f.documentInfo = [kCGPDFContextTitle as String: title, kCGPDFContextCreator as String: "Tailor It"]
        return f
    }
}
