import UIKit
import UniformTypeIdentifiers

/// 共有シートの受け口(#2224)。受けた物を App Group の `ShareInbox` に置き、`tailorit://share-inbox` で本体を開く。
/// 画面は出さない(置いたらすぐ閉じる)。振り分けは本体側・#1775 の決定のあと。
final class ShareViewController: UIViewController {
    override func viewDidLoad() {
        super.viewDidLoad()
        view.backgroundColor = .clear
        Task { await receive() }
    }

    private func receive() async {
        let providers = ((extensionContext?.inputItems as? [NSExtensionItem]) ?? []).flatMap { $0.attachments ?? [] }
        if let inbox = ShareInbox.shared {
            for provider in providers {
                await Self.save(provider, to: inbox)
            }
        }
        extensionContext?.open(ShareInbox.openURL, completionHandler: nil)
        extensionContext?.completeRequest(returningItems: [])
    }

    /// 1 件の添付を PDF → 画像 → URL → 文字の順に見て、最初に合った形で置く。
    private static func save(_ provider: NSItemProvider, to inbox: ShareInbox) async {
        if provider.hasItemConformingToTypeIdentifier(UTType.pdf.identifier) {
            guard let data = await provider.data(ofType: .pdf) else { return }
            try? inbox.put(kind: .pdf, data: data, fileExtension: "pdf")
        } else if let type = provider.firstType(conformingTo: .image) {
            guard let data = await provider.data(ofType: type) else { return }
            try? inbox.put(kind: .image, data: data, fileExtension: type.preferredFilenameExtension ?? "img")
        } else if provider.hasItemConformingToTypeIdentifier(UTType.url.identifier) {
            guard let text = await provider.text(ofType: .url) else { return }
            try? inbox.put(kind: .url, text: text)
        } else if provider.hasItemConformingToTypeIdentifier(UTType.plainText.identifier) {
            guard let text = await provider.text(ofType: .plainText) else { return }
            try? inbox.put(kind: .text, text: text)
        }
    }
}

private extension NSItemProvider {
    /// 登録されている型のうち、指定の型に合う最初の 1 つ(画像は jpeg / png などの具体的な型で来るため)
    func firstType(conformingTo type: UTType) -> UTType? {
        registeredTypeIdentifiers.compactMap(UTType.init(_:)).first { $0.conforms(to: type) }
    }

    func data(ofType type: UTType) async -> Data? {
        await withCheckedContinuation { continuation in
            _ = loadDataRepresentation(forTypeIdentifier: type.identifier) { data, _ in
                continuation.resume(returning: data)
            }
        }
    }

    func text(ofType type: UTType) async -> String? {
        await withCheckedContinuation { continuation in
            loadItem(forTypeIdentifier: type.identifier, options: nil) { value, _ in
                switch value {
                case let url as URL: continuation.resume(returning: url.absoluteString)
                case let string as String: continuation.resume(returning: string)
                case let data as Data: continuation.resume(returning: String(data: data, encoding: .utf8))
                default: continuation.resume(returning: nil)
                }
            }
        }
    }
}
