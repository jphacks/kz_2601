import SwiftUI
import UIKit
import AVFoundation
import UniformTypeIdentifiers
import CoreTransferable

// 審査用ビルド(APP_STORE_REVIEW・#245)では撮影をバイナリから外す(ライブラリから選ぶ PickedMovie は残す)。
#if !APP_STORE_REVIEW
/// 動画撮影の `UIImagePickerController`(#206)。標準のカメラ UI に内カメ / 外カメの切り替えボタンがある。
/// 録画の上限は `maxDuration` 秒、画質は中程度(`.typeMedium`)。権限は呼び出し側で `requestAccess()` を先に確認する。
struct VideoCameraPicker: UIViewControllerRepresentable {
    var maxDuration: TimeInterval
    var onVideo: (URL) -> Void
    @Environment(\.dismiss) private var dismiss

    static var isAvailable: Bool {
        UIImagePickerController.isSourceTypeAvailable(.camera)
            && (UIImagePickerController.availableMediaTypes(for: .camera) ?? []).contains(UTType.movie.identifier)
    }

    /// カメラは必須。マイクは拒否されても無音で撮れるので結果を問わない(未決定ならここで尋ねる)。
    static func requestAccess() async -> Bool {
        let camera = await CameraPicker.requestAccess()
        if AVCaptureDevice.authorizationStatus(for: .audio) == .notDetermined {
            _ = await AVCaptureDevice.requestAccess(for: .audio)
        }
        return camera
    }

    func makeCoordinator() -> Coordinator { Coordinator(self) }

    func makeUIViewController(context: Context) -> UIImagePickerController {
        let picker = UIImagePickerController()
        picker.sourceType = .camera
        picker.mediaTypes = [UTType.movie.identifier]
        picker.cameraCaptureMode = .video
        picker.cameraDevice = .rear
        picker.videoMaximumDuration = maxDuration
        picker.videoQuality = .typeMedium
        picker.allowsEditing = false
        picker.delegate = context.coordinator
        return picker
    }

    func updateUIViewController(_ uiViewController: UIImagePickerController, context: Context) {}

    final class Coordinator: NSObject, UIImagePickerControllerDelegate, UINavigationControllerDelegate {
        let parent: VideoCameraPicker
        init(_ parent: VideoCameraPicker) { self.parent = parent }

        func imagePickerController(_ picker: UIImagePickerController, didFinishPickingMediaWithInfo info: [UIImagePickerController.InfoKey: Any]) {
            if let url = info[.mediaURL] as? URL { parent.onVideo(url) }
            parent.dismiss()
        }

        func imagePickerControllerDidCancel(_ picker: UIImagePickerController) {
            parent.dismiss()
        }
    }
}
#endif

/// PhotosPicker で選んだ動画を一時ファイルとして受け取る。
struct PickedMovie: Transferable {
    let url: URL

    static var transferRepresentation: some TransferRepresentation {
        FileRepresentation(contentType: .movie) { SentTransferredFile($0.url) } importing: { received in
            let ext = received.file.pathExtension.isEmpty ? "mov" : received.file.pathExtension
            let dest = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString).appendingPathExtension(ext)
            try FileManager.default.copyItem(at: received.file, to: dest)
            return Self(url: dest)
        }
    }
}
