import SwiftUI
import MetamonDSL

extension Component {
    /// container の `spacing`(none|small|medium|large)→ pt。
    var containerSpacing: CGFloat {
        switch spacing { case "none": 0; case "small": 6; case "large": 24; default: 12 }
    }

    /// 自分でカード(面)を描く部品。section がこれだけを子に持つ時は二重カードにしない。
    var rendersOwnSurface: Bool {
        switch type {
        case .list, .table, .timeline, .card, .stat, .chart, .calendar, .progressBar, .gauge, .timerDisplay, .disclosure, .carousel, .mapView, .emptyState: true
        default: false
        }
    }
}
