import SwiftUI
import MapKit
import MetamonDSL

/// `mapView`: MapKit。`entity.locationField`("lat,lng")を持つレコードごとにピン。ピンのタップで `onItemTap`(recordId 付き)。
/// `zoom`: near(約1km) / city(約10km・既定) / wide(約100km)。ピンが無ければ東京駅を中心に出す。
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

    var body: some View {
        let pins = pins
        Map(position: $position) {
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
