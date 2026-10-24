import Foundation

/// `photoField.source`(camera|library|both・省略は both)。
enum PhotoSourceMode: String {
    case camera, library, both

    init(raw: String?) {
        switch raw?.lowercased() {
        case "camera": self = .camera
        case "library": self = .library
        default: self = .both   // "both" / "any" / 省略 / 未知
        }
    }
}

/// タップ時に実際に開くもの。
enum PhotoCapturePlan: Equatable {
    case camera          // UIImagePickerController(.camera)
    case library         // PhotosPicker
    case choose          // 「撮影」「写真から選ぶ」の confirmationDialog
}

/// source と端末のカメラ可否から開き方を決める(純関数・テスト対象)。
/// カメラが無い(シミュレータ等)なら camera/both は写真選択に落とす。
enum PhotoSourcePolicy {
    static func plan(for mode: PhotoSourceMode, cameraAvailable: Bool) -> PhotoCapturePlan {
        guard cameraAvailable else { return .library }
        switch mode {
        case .camera: return .camera
        case .library: return .library
        case .both: return .choose
        }
    }

    static func plan(source raw: String?, cameraAvailable: Bool) -> PhotoCapturePlan {
        plan(for: PhotoSourceMode(raw: raw), cameraAvailable: cameraAvailable)
    }
}
