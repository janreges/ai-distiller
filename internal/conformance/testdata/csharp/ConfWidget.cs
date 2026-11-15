using System.IO;

// CONF_LINE_COMMENT

namespace Conf;

/// <summary>CONF_DOC_COMMENT</summary>
public class ConfWidget {
    public int ConfField = 1;

    public string ConfPublic() {
        string a = "CONF_BODY_A";
        return a;
    }

    /// <summary>CONF_PRIVATE_DOC</summary>
    private string ConfPrivate() {
        return Path.GetTempPath();
    }

    protected internal string ConfProtInt() { return "x"; }

    public string ConfOther() {
        string b = "CONF_BODY_B";
        return b;
    }
}
