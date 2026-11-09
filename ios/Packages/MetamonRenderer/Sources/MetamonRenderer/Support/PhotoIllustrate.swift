import SwiftUI
import MetamonDSL
#if canImport(ImagePlayground) && !APP_STORE_REVIEW
import ImagePlayground
#endif

/// `photoField.illustrate`: 端末内の Image Playground でイラストを作り、選んだ写真と同じ形で photo field に入れる。
/// クラウドでは作らない。使えない端末・審査用の版ではボタンを出さない。
enum PhotoIllustrate {
    /// 最初の案に渡す文字の上限(長い本文をそのまま渡さない)。
    static let maxConceptLength = 80

    /// ボタンを出すか(純関数・テスト対象)。
    static func showsButton(requested: Bool, available: Bool) -> Bool { requested && available }

    /// 最初の案: titleField(string/text)の値 → 見える保存 field のうち最初の空でない string/text。無ければ空。
    static func concept(entity: Entity, values: [String: JSONValue]) -> String {
        let candidates = entity.storedFields.filter { !$0.hidden && ($0.type == .string || $0.type == .text) }
        let ordered = candidates.filter { $0.id == entity.titleField } + candidates.filter { $0.id != entity.titleField }
        for f in ordered {
            let text = (values[f.id]?.stringValue ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
            if !text.isEmpty { return String(text.prefix(maxConceptLength)) }
        }
        return ""
    }

    /// Image Playground が返したファイルを、選んだ写真と同じ data URL(800px 以下・JPEG)にする。
    static func dataURL(fromFileAt url: URL) -> String? {
        guard let data = try? Data(contentsOf: url) else { return nil }
        return PhotoCodec.dataURL(from: data)
    }

    /// この端末で Image Playground が使えるか(iOS 18.1 以上・Apple Intelligence)。審査用の版は常に false。
    @MainActor static var isAvailable: Bool {
        #if canImport(ImagePlayground) && !APP_STORE_REVIEW
        if #available(iOS 18.1, *) { return ImagePlaygroundViewController.isAvailable }
        #endif
        return false
    }
}

/// Image Playground のシートを付ける(使えない OS では何もしない)。
struct IllustrateSheet: ViewModifier {
    @Binding var isPresented: Bool
    let concept: String
    let onImage: (URL) -> Void

    func body(content: Content) -> some View {
        #if canImport(ImagePlayground) && !APP_STORE_REVIEW
        if #available(iOS 18.1, *) {
            content.imagePlaygroundSheet(isPresented: $isPresented, concept: concept, onCompletion: onImage)
        } else {
            content
        }
        #else
        content
        #endif
    }
}
