import SwiftUI
import MapKit
import MetamonDSL

/// `mapView`: MapKit。`entity.locationField`("lat,lng")を持つレコードごとにピン。ピンのタップで `onItemTap`(recordId 付き)。
/// `zoom`: near(約1km) / city(約10km・既定) / wide(約100km)。ピンが無ければ東京駅を中心に出す。
/// `kind: "route"`(#168): ピンの代わりに点を `dateField`(省略時は作成順)の時刻順に線で結ぶ(`locationTracker` の軌跡)。
/// 10 分以上空いた所は別の記録として線を切る。始点・終点だけ印を出し、カメラは最新の記録に合わせる。
struct MapViewLeaf: View {
    @Environment(\.dslRuntime) private var rt
    let c: Component
    let ctx: RenderContext
    @State private var position: MapCameraPosition = .automatic

    private struct Pin: Identifiable {
        let id: String
        let title: String
        let coordinate: CLLocationCoordinate2D
    }

    private var pins: [Pin] {
        let entityId = c.entity ?? ""
        let entity = rt.dsl.entity(entityId)
        let titleField = c.titleField ?? entity?.titleField
        return rt.viewRecords(for: entityId).compactMap { r in
            guard let coord = Self.coordinate(r[c.locationField ?? ""]) else { return nil }
            let title = titleField.map { rt.display(r[$0], field: entity?.field($0)) } ?? ""
            return Pin(id: r.id, title: title, coordinate: coord)
        }
    }

    private var spanMeters: CLLocationDistance {
        switch c.zoom { case "near": 1_000; case "wide": 100_000; default: 10_000 }
    }

    private var isRoute: Bool { c.kind == "route" }

    /// route: 時刻順に並べて 10 分の空きで切った点列
    private var segments: [[Track.Point]] {
        Track.segments(Track.points(rt.viewRecords(for: c.entity ?? ""), locationField: c.locationField ?? "", by: c.dateField))
    }

    var body: some View {
        if isRoute { routeMap } else { pinMap }
    }

    private var routeMap: some View {
        let segs = segments
        let latest = segs.last ?? []
        return Map(position: $position) {
            ForEach(Array(segs.enumerated()), id: \.offset) { i, seg in
                MapPolyline(coordinates: seg.map(Self.coord))
                    .stroke(i == segs.count - 1 ? AnyShapeStyle(.tint) : AnyShapeStyle(.tint.opacity(0.4)),
                            style: StrokeStyle(lineWidth: 5, lineCap: .round, lineJoin: .round))
            }
            if let first = latest.first {
                Marker(L("スタート"), systemImage: "figure.walk", coordinate: Self.coord(first)).tint(.green)
            }
            if latest.count > 1, let last = latest.last {
                Marker(L("いま / ゴール"), systemImage: "flag.checkered", coordinate: Self.coord(last))
            }
        }
        .mapStyle(.standard(pointsOfInterest: .excludingAll))
        .frame(maxWidth: .infinity, minHeight: 160, maxHeight: .infinity)
        .clipShape(RoundedRectangle(cornerRadius: RenderTheme.Radius.card, style: .continuous))
        .onAppear { position = Self.fit(latest.map(Self.coord), minSpan: routeMinSpan) }
        .onChange(of: latest.count) { _, _ in position = Self.fit(latest.map(Self.coord), minSpan: routeMinSpan) }
    }

    /// route の最小の広さ: zoom 省略時は約 500 m(散歩の線が見える)
    private var routeMinSpan: CLLocationDistance { c.zoom == nil ? 500 : spanMeters }

    private static func coord(_ p: Track.Point) -> CLLocationCoordinate2D {
        CLLocationCoordinate2D(latitude: p.lat, longitude: p.lng)
    }

    /// 点列全体が入る範囲(余白 1.4 倍)。`minSpan` より狭くはしない(near 指定なら約 1km)。
    static func fit(_ coords: [CLLocationCoordinate2D], minSpan: CLLocationDistance) -> MapCameraPosition {
        guard let first = coords.first else { return camera(for: [], span: minSpan) }
        var minLat = first.latitude, maxLat = first.latitude, minLng = first.longitude, maxLng = first.longitude
        for c in coords {
            minLat = min(minLat, c.latitude); maxLat = max(maxLat, c.latitude)
            minLng = min(minLng, c.longitude); maxLng = max(maxLng, c.longitude)
        }
        let center = CLLocationCoordinate2D(latitude: (minLat + maxLat) / 2, longitude: (minLng + maxLng) / 2)
        let latMeters = (maxLat - minLat) * 111_000 * 1.4
        let lngMeters = (maxLng - minLng) * 111_000 * cos(center.latitude * .pi / 180) * 1.4
        let span = max(minSpan, latMeters, lngMeters)
        return .region(MKCoordinateRegion(center: center, latitudinalMeters: span, longitudinalMeters: span))
    }

    private var pinMap: some View {
        let pins = pins
        return Map(position: $position) {
            ForEach(pins) { pin in
                Annotation(pin.title, coordinate: pin.coordinate) {
                    Button {
                        guard let action = c.onItemTap else { return }
                        Task { await rt.run(action, ctx.actionContext(recordId: pin.id)) }
                    } label: {
                        Image(systemName: "mappin.circle.fill")
                            .font(.title)
                            .foregroundStyle(.white, .tint)
                    }
                    .buttonStyle(.plain)
                }
            }
        }
        .mapStyle(.standard(pointsOfInterest: .excludingAll))
        .frame(maxWidth: .infinity, minHeight: 160, maxHeight: .infinity)
        .clipShape(RoundedRectangle(cornerRadius: RenderTheme.Radius.card, style: .continuous))
        .onAppear { position = Self.camera(for: pins.map(\.coordinate), span: spanMeters) }
        .onChange(of: pins.map(\.id)) { _, _ in position = Self.camera(for: pins.map(\.coordinate), span: spanMeters) }
    }

    /// "lat,lng" → 座標
    static func coordinate(_ v: JSONValue) -> CLLocationCoordinate2D? {
        guard let s = v.stringValue else { return nil }
        let parts = s.split(separator: ",").compactMap { Double($0.trimmingCharacters(in: .whitespaces)) }
        guard parts.count == 2, (-90...90).contains(parts[0]), (-180...180).contains(parts[1]) else { return nil }
        return CLLocationCoordinate2D(latitude: parts[0], longitude: parts[1])
    }

    private static func camera(for coords: [CLLocationCoordinate2D], span: CLLocationDistance) -> MapCameraPosition {
        guard !coords.isEmpty else {
            return .region(MKCoordinateRegion(center: CLLocationCoordinate2D(latitude: 35.6812, longitude: 139.7671),
                                              latitudinalMeters: span, longitudinalMeters: span))
        }
        let lat = coords.map(\.latitude).reduce(0, +) / Double(coords.count)
        let lng = coords.map(\.longitude).reduce(0, +) / Double(coords.count)
        return .region(MKCoordinateRegion(center: CLLocationCoordinate2D(latitude: lat, longitude: lng),
                                          latitudinalMeters: span, longitudinalMeters: span))
    }
}
