import SwiftUI
import MapKit
#if os(macOS)
import AppKit
#endif

// 画面の中の地図(mapView・placeSearch)の Mac の操作(#1346)。iOS は今までと同じ。

enum PlatformMap {
    /// 地図の操作。Mac はホイール・2 本指のスクロールを画面の送りに残すため、ドラッグ / スクロールの移動を外す
    /// (拡大は +/- とピンチ)。iOS は渡したまま。
    static func inlineModes(_ modes: MapInteractionModes) -> MapInteractionModes {
        #if os(macOS)
        modes.subtracting(.pan)
        #else
        modes
        #endif
    }
}

extension View {
    /// Mac: ズームの +/-・方位・縮尺を出す。iOS は何もしない(見た目を変えない)。
    func macMapControls() -> some View {
        #if os(macOS)
        mapControls {
            MapZoomStepper()
            MapCompass()
            MapScaleView()
        }
        #else
        self
        #endif
    }

    /// Mac: ホバー中はポインタを指の形にする(押せる印)。iOS は何もしない。
    func macPointingHandCursor() -> some View {
        modifier(MacPointingHandCursor())
    }
}

private struct MacPointingHandCursor: ViewModifier {
    #if os(macOS)
    @State private var pushed = false
    #endif

    func body(content: Content) -> some View {
        #if os(macOS)
        content
            .onHover { inside in
                if inside, !pushed { NSCursor.pointingHand.push(); pushed = true }
                if !inside, pushed { NSCursor.pop(); pushed = false }
            }
            .onDisappear { if pushed { NSCursor.pop(); pushed = false } }
        #else
        content
        #endif
    }
}
