import SwiftUI
import UniformTypeIdentifiers

/// 画像の「コピー・画像を保存…・共有」(#1333)。保存するデータとファイル名を決める。
enum ImageExport {
    /// 保存できる形(先頭が既定)。
    static let contentTypes: [UTType] = [.png, .jpeg]

    /// 画像を保存する形のデータにする。JPEG / PNG 以外は nil。
    static func data(from image: PlatformImage, as type: UTType) -> Data? {
        if type.conforms(to: .png) { return image.pngData() }
        if type.conforms(to: .jpeg) { return image.jpegData(compressionQuality: 0.9) }
        return nil
    }

    /// 既定のファイル名(拡張子なし)。「<名前> 2026-11-11 10.20.30」。
    /// ファイル名に使えない `/` `:` は「-」に、改行は空白にし、先頭の「.」(隠しファイル)を除き、名前は 60 文字まで。
    static func fileName(title: String?, fallback: String, date: Date, timeZone: TimeZone = .current) -> String {
        let cleaned = (title ?? "")
            .replacingOccurrences(of: "/", with: "-")
            .replacingOccurrences(of: ":", with: "-")
            .components(separatedBy: .newlines).joined(separator: " ")
            .trimmingCharacters(in: .whitespaces.union(CharacterSet(charactersIn: ".")))
        let name = cleaned.isEmpty ? fallback : String(cleaned.prefix(60))
        var cal = Calendar(identifier: .gregorian)
        cal.timeZone = timeZone
        let d = cal.dateComponents([.year, .month, .day, .hour, .minute, .second], from: date)
        let stamp = String(format: "%04d-%02d-%02d %02d.%02d.%02d", d.year ?? 0, d.month ?? 0, d.day ?? 0, d.hour ?? 0, d.minute ?? 0, d.second ?? 0)
        return "\(name) \(stamp)"
    }

    /// 保存待ちの画像(`.fileExporter` に渡す)。
    struct Pending: Identifiable {
        let id = UUID()
        let document: ImageFileDocument
        let fileName: String
    }
}

/// `.fileExporter` 用。保存パネルで選んだ形(PNG / JPEG)で書く。
struct ImageFileDocument: FileDocument {
    static var readableContentTypes: [UTType] { ImageExport.contentTypes }

    let png: Data?
    let jpeg: Data?

    init(image: PlatformImage) {
        png = ImageExport.data(from: image, as: .png)
        jpeg = ImageExport.data(from: image, as: .jpeg)
    }

    init(configuration: ReadConfiguration) throws {
        guard let data = configuration.file.regularFileContents else { throw CocoaError(.fileReadCorruptFile) }
        png = data
        jpeg = data
    }

    func fileWrapper(configuration: WriteConfiguration) throws -> FileWrapper {
        guard let data = configuration.contentType.conforms(to: .jpeg) ? jpeg : png else {
            throw CocoaError(.fileWriteUnknown)
        }
        return FileWrapper(regularFileWithContents: data)
    }
}

#if os(macOS)
/// 画像の右クリックの項目(Mac のみ)。並びは「コピー → 画像を保存… → 共有」(docs/mac/interactions.md)。
struct ImageMenuItems: View {
    let image: PlatformImage
    let title: String
    /// 一覧の行のように別の「コピー」と並ぶ時は「画像をコピー」にする
    var copyLabel: String = L("コピー")
    let onSave: () -> Void

    var body: some View {
        Button(copyLabel, systemImage: "doc.on.doc") { PlatformPasteboard.setImage(image) }
        Button(L("画像を保存…"), systemImage: "square.and.arrow.down", action: onSave)
        ShareLink(item: Image(platformImage: image), preview: SharePreview(title, image: Image(platformImage: image))) {
            Label(L("共有"), systemImage: "square.and.arrow.up")
        }
    }
}

extension View {
    /// 保存待ちの画像があれば保存パネルを出す。
    func imageSaveExporter(_ pending: Binding<ImageExport.Pending?>) -> some View {
        fileExporter(
            isPresented: Binding(get: { pending.wrappedValue != nil }, set: { if !$0 { pending.wrappedValue = nil } }),
            document: pending.wrappedValue?.document,
            contentTypes: ImageExport.contentTypes,
            defaultFilename: pending.wrappedValue?.fileName
        ) { _ in pending.wrappedValue = nil }
    }
}

/// 画像に右クリックの「コピー・画像を保存…・共有」を付ける(画像が無ければ何も付けない)。
private struct ImageContextMenu: ViewModifier {
    let image: PlatformImage?
    let title: String
    @State private var pending: ImageExport.Pending?

    func body(content: Content) -> some View {
        if let image {
            content
                .contextMenu {
                    ComponentTalkImageMenuItem() // 画像のメニューが先に出るので、ここにも置く(#1323)
                    ImageMenuItems(image: image, title: title) {
                        pending = .init(document: ImageFileDocument(image: image),
                                        fileName: ImageExport.fileName(title: title, fallback: L("写真"), date: Date()))
                    }
                }
                .imageSaveExporter($pending)
        } else {
            content
        }
    }
}
#endif

extension View {
    /// Mac: 画像の右クリックに「コピー・画像を保存…・共有」(#1333)。iOS は何もしない(長押しの動きは変えない)。
    @ViewBuilder
    func imageContextMenu(_ image: PlatformImage?, title: String) -> some View {
        #if os(macOS)
        modifier(ImageContextMenu(image: image, title: title))
        #else
        self
        #endif
    }
}
