import SwiftUI
import MetamonDSL

extension Component {
    /// container の `spacing`(none|small|medium|large)→ pt。
    var containerSpacing: CGFloat {
        switch spacing { case "none": 0; case "small": 6; case "large": 24; default: 12 }
    }
}
