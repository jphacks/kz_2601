import Foundation
import CoreLocation

/// 現在地の精度を「正確 / おおよそ」に分ける(#1344)。
/// Mac は Wi-Fi の位置で誤差が大きい。iOS も「正確な位置」がオフだと落ちるので、表示は共通。
/// 保存する値("lat,lng")は変えず、画面に一言出すだけ。
enum LocationAccuracy: Equatable {
    case precise
    case approximate(meters: CLLocationDistance)

    /// これより誤差が大きければ「おおよそ」(m)。`desiredAccuracy` の 100m に合わせる。
    static let approximateThreshold: CLLocationDistance = 100

    /// `CLLocation.horizontalAccuracy` から分ける。負の値(不明)はおおよそ扱いにせず正確とみなす。
    static func classify(_ horizontalAccuracy: CLLocationDistance) -> LocationAccuracy {
        guard horizontalAccuracy.isFinite, horizontalAccuracy > approximateThreshold else { return .precise }
        return .approximate(meters: horizontalAccuracy)
    }

    /// 「おおよその位置(±1.2 km)」。正確なら nil。
    var note: String? {
        guard case .approximate(let m) = self else { return nil }
        let radius = PlaceSearch.distanceText(m)
        return L("おおよその位置(±\(radius))")
    }

    /// locationPicker の「現在地」用: ピンで直せることも添える。
    var pickerNote: String? {
        guard case .approximate(let m) = self else { return nil }
        let radius = PlaceSearch.distanceText(m)
        return L("おおよその位置(±\(radius))です。地図を動かしてピンで直せます")
    }

    /// placeSearch 用: 距離が当てにならないことを伝える。
    var searchNote: String? {
        guard case .approximate(let m) = self else { return nil }
        let radius = PlaceSearch.distanceText(m)
        return L("おおよその位置(±\(radius))から探しました。距離は目安です")
    }
}
