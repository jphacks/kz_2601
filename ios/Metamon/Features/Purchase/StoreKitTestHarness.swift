#if DEBUG
import Foundation

/// ヘッドレス確認用(DEBUG・シミュレータのみ): `METAMON_STOREKIT_CONFIG=<.storekit の絶対パス>` で起動すると、
/// アプリのプロセス内で `SKTestSession` を開き、ローカル構成で購入できるようにする。
/// Xcode の Run は Scheme の StoreKit Configuration が効くので不要。`simctl launch` で起動した時だけ使う(#77)。
/// StoreKitTest はテスト用フレームワークでアプリにはリンクしないため、実行時に dlopen + ObjC ランタイムで呼ぶ。
enum StoreKitTestHarness {
    nonisolated(unsafe) private static var session: AnyObject?

    static func startIfRequested() {
        #if targetEnvironment(simulator)
        guard session == nil, let path = ProcessInfo.processInfo.environment["METAMON_STOREKIT_CONFIG"], !path.isEmpty else { return }
        guard dlopen("/Developer/Library/Frameworks/StoreKitTest.framework/StoreKitTest", RTLD_NOW) != nil,
              let cls = NSClassFromString("SKTestSession") as? NSObject.Type else {
            print("[Metamon] StoreKitTest: framework not loadable")
            return
        }
        typealias Init = @convention(c) (AnyObject, Selector, NSURL, UnsafeMutablePointer<NSError?>?) -> AnyObject?
        let sel = NSSelectorFromString("initWithContentsOfURL:error:")
        guard let alloc = cls.perform(NSSelectorFromString("alloc"))?.takeUnretainedValue(),
              let imp = (alloc as? NSObject)?.method(for: sel) else { return }
        var error: NSError?
        let initFn = unsafeBitCast(imp, to: Init.self)
        guard let s = initFn(alloc, sel, URL(fileURLWithPath: path) as NSURL, &error) as? NSObject else {
            print("[Metamon] StoreKitTest: session failed \(error.map { "\($0)" } ?? "-")")
            return
        }
        s.setValue(true, forKey: "disableDialogs")
        if ProcessInfo.processInfo.environment["METAMON_STOREKIT_RESET"] == "1" {
            _ = s.perform(NSSelectorFromString("resetToDefaultState"))
            _ = s.perform(NSSelectorFromString("clearTransactions"))
        }
        session = s
        print("[Metamon] StoreKitTest: session started with \(path)")
        #endif
    }
}
#endif
