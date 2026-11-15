import Foundation

// CONF_LINE_COMMENT

/// CONF_DOC_COMMENT
public class ConfWidget {
    public var confField: Int = 1

    public func confPublic() -> String {
        let a = "CONF_BODY_A"
        return a
    }

    /// CONF_PRIVATE_DOC
    private func confPrivate() -> String {
        return NSTemporaryDirectory()
    }

    public func confOther() -> String {
        let b = "CONF_BODY_B"
        return b
    }
}

public struct ConfPoint {
    public var confX: Int = 0
}
