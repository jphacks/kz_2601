import SwiftUI
import Photos
import MetamonDSL

/// `qrCode`(#422): text / field / value から QR を作って表示する(作り方は `QRCodeImage`・中身は `QRCodeContent`)。
/// 長押しのメニューと下のボタンで写真に保存(追加のみの権限)・共有シート。空・長すぎる時は案内を出す。
/// ダークモードでも白地に黒(読み取りやすさ優先)。
struct QrCodeLeaf: View {
    @Environment(\.dslRuntime) private var rt
    let c: Component
    let ctx: RenderContext

    enum SaveState: Equatable { case idle, saving, saved, failed }
    @State private var saveState: SaveState = .idle

    var body: some View {
        let content = QRCodeContent.resolve(c, form: ctx.form) { rt.evaluator() }
        VStack(spacing: RenderTheme.Spacing.sm) {
            if let title = c.title, !title.isEmpty {
                Text(title)
                    .font(RenderTheme.Fonts.sectionTitle)
                    .foregroundStyle(.secondary)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .accessibilityAddTraits(.isHeader)
            }
            if content.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                placeholder(L("QR にする内容がまだありません"))
            } else if let shown = QRCodeImage.image(for: content, scale: 8), let export = QRCodeImage.image(for: content) {
                code(shown, export: export, content: content)
            } else {
                placeholder(L("長すぎて QR コードにできません"))
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .onChange(of: content) { saveState = .idle }
    }

    private var caption: String? { c.label.flatMap { $0.isEmpty ? nil : $0 } }

    @ViewBuilder
    private func code(_ shown: UIImage, export: UIImage, content: String) -> some View {
        Image(uiImage: shown)
            .interpolation(.none)
            .resizable()
            .scaledToFit()
            .frame(maxWidth: 320, maxHeight: .infinity)
            .clipShape(RoundedRectangle(cornerRadius: RenderTheme.Radius.inner, style: .continuous))
            .overlay(RoundedRectangle(cornerRadius: RenderTheme.Radius.inner, style: .continuous).strokeBorder(RenderTheme.Colors.hairline))
            .contentShape(.contextMenuPreview, RoundedRectangle(cornerRadius: RenderTheme.Radius.inner, style: .continuous))
            .contextMenu {
                Button { save(export) } label: { Label(L("写真に保存"), systemImage: "square.and.arrow.down") }
                ShareLink(item: Image(uiImage: export), preview: SharePreview(caption ?? L("QR コード"), image: Image(uiImage: export))) {
                    Label(L("共有"), systemImage: "square.and.arrow.up")
                }
            }
            .accessibilityElement()
            .accessibilityLabel(caption ?? L("QR コード"))
            .accessibilityValue(content)
            .accessibilityIdentifier("qrCode.image")
            .layoutPriority(1)
        if let caption {
            Text(caption)
                .font(.footnote)
                .foregroundStyle(.secondary)
                .multilineTextAlignment(.center)
                .lineLimit(2)
                .accessibilityHidden(true)
        }
        HStack(spacing: RenderTheme.Spacing.sm) {
            Button { save(export) } label: {
                Label(saveLabel, systemImage: saveState == .saved ? "checkmark" : "square.and.arrow.down")
            }
            .disabled(saveState == .saving)
            .accessibilityIdentifier("qrCode.save")
            ShareLink(item: Image(uiImage: export), preview: SharePreview(caption ?? L("QR コード"), image: Image(uiImage: export))) {
                Label(L("共有"), systemImage: "square.and.arrow.up")
            }
            .accessibilityIdentifier("qrCode.share")
        }
        .font(.footnote.weight(.semibold))
        .buttonStyle(.bordered)
        .controlSize(.small)
        .sensoryFeedback(.success, trigger: saveState == .saved)
    }

    private var saveLabel: String {
        switch saveState {
        case .saved: L("写真に保存しました")
        case .failed: L("保存できませんでした")
        default: L("写真に保存")
        }
    }

    private func placeholder(_ message: String) -> some View {
        VStack(spacing: RenderTheme.Spacing.sm) {
            Image(systemName: "qrcode")
                .font(.system(size: 34, weight: .regular))
                .foregroundStyle(.tertiary)
                .accessibilityHidden(true)
            Text(message)
                .font(.subheadline)
                .foregroundStyle(.secondary)
                .multilineTextAlignment(.center)
        }
        .frame(maxWidth: .infinity, minHeight: 120)
        .background(RenderTheme.Colors.surface, in: RoundedRectangle(cornerRadius: RenderTheme.Radius.inner, style: .continuous))
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("qrCode.placeholder")
    }

    /// 写真に追加(`NSPhotoLibraryAddUsageDescription`・追加のみ)。拒否・失敗は文言で知らせる。
    private func save(_ image: UIImage) {
        saveState = .saving
        Task { @MainActor in
            let status = await PHPhotoLibrary.requestAuthorization(for: .addOnly)
            guard status == .authorized || status == .limited else {
                saveState = .failed
                return
            }
            do {
                try await PHPhotoLibrary.shared().performChanges {
                    PHAssetChangeRequest.creationRequestForAsset(from: image)
                }
                saveState = .saved
            } catch {
                saveState = .failed
            }
        }
    }
}
