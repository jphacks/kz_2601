import MetamonDSL

/// iOS の版で使える機能(#415)。Mac と共有する PromptTips.swift から分けた(#967・FirstRunTutorial が iOS だけにあるため)。
extension PromptTips.Availability {
    static var current: PromptTips.Availability {
        from(allowedComponents: ClientCapabilities.allowedComponents,
             disabledFeatures: ClientCapabilities.disabledFeatures,
             sharing: FirstRunTutorial.sharingAvailable)
    }

    static var review: PromptTips.Availability {
        from(allowedComponents: ClientCapabilities.reviewAllowedComponents,
             disabledFeatures: ClientCapabilities.reviewDisabledFeatures, sharing: false)
    }
}
