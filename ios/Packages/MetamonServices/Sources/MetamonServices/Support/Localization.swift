import Foundation
import MetamonDSL

/// Services 内の固定文言(`Bundle.module` の String Catalog)。言語はアプリ内の設定に従う。
func L(_ key: String.LocalizationValue) -> String {
    PackageLocalization.string(key, bundle: .module)
}
