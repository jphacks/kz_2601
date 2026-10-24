import SwiftUI
import CoreLocation
import MetamonDSL

/// `locationButton`: CoreLocation で現在地を 1 回取得し、フォームの location field に "lat,lng" を保存する。
/// 権限は `NSLocationWhenInUseUsageDescription`(Info.plist 設定済み)。シミュレータは Features > Location で位置を与える。
struct LocationButtonLeaf: View {
    let c: Component
    let ctx: RenderContext
    @State private var busy = false
    @State private var error: String?

    var body: some View {
        if let form = ctx.form, let field = c.field {
            VStack(alignment: .leading, spacing: 4) {
                Button {
                    Task { await capture(into: form, field: field) }
                } label: {
                    HStack {
                        if busy { ProgressView().controlSize(.small) }
                        Label(c.label ?? "現在地を記録", systemImage: "location.fill")
                    }
                    .frame(maxWidth: .infinity)
                }
                .buttonStyle(.bordered)
                .controlSize(.large)
                .disabled(busy)
                if let s = (form.values[field] ?? .null).stringValue, !s.isEmpty {
                    Text("📍 \(s)").font(.caption).foregroundStyle(.secondary).monospacedDigit()
                } else if let error {
                    Text(error).font(.caption).foregroundStyle(.red)
                }
            }
        } else {
            NeedsForm(c: c)
        }
    }

    private func capture(into form: FormDraft, field: String) async {
        busy = true
        defer { busy = false }
        do {
            let loc = try await OneShotLocation().request()
            form.values[field] = .string(String(format: "%.6f,%.6f", loc.coordinate.latitude, loc.coordinate.longitude))
            error = nil
        } catch {
            self.error = "位置情報を取得できませんでした(\(error.localizedDescription))"
        }
    }
}

/// CLLocationManager を async 1 回呼びに包む。
@MainActor
final class OneShotLocation: NSObject, CLLocationManagerDelegate {
    private let manager = CLLocationManager()
    private var continuation: CheckedContinuation<CLLocation, Error>?

    enum LocationError: LocalizedError {
        case denied
        var errorDescription: String? { "位置情報の利用が許可されていません" }
    }

    func request() async throws -> CLLocation {
        manager.delegate = self
        manager.desiredAccuracy = kCLLocationAccuracyHundredMeters
        return try await withCheckedThrowingContinuation { cont in
            continuation = cont
            switch manager.authorizationStatus {
            case .notDetermined: manager.requestWhenInUseAuthorization()
            case .denied, .restricted: finish(.failure(LocationError.denied))
            default: manager.requestLocation()
            }
        }
    }

    private func finish(_ result: Result<CLLocation, Error>) {
        continuation?.resume(with: result)
        continuation = nil
    }

    nonisolated func locationManagerDidChangeAuthorization(_ manager: CLLocationManager) {
        let status = manager.authorizationStatus
        Task { @MainActor in
            guard continuation != nil else { return }
            switch status {
            case .authorizedWhenInUse, .authorizedAlways: self.manager.requestLocation()
            case .denied, .restricted: finish(.failure(LocationError.denied))
            default: break
            }
        }
    }

    nonisolated func locationManager(_ manager: CLLocationManager, didUpdateLocations locations: [CLLocation]) {
        guard let loc = locations.last else { return }
        Task { @MainActor in finish(.success(loc)) }
    }

    nonisolated func locationManager(_ manager: CLLocationManager, didFailWithError error: Error) {
        Task { @MainActor in finish(.failure(error)) }
    }
}
