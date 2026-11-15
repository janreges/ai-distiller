package com.example.conf;

import java.io.File;

// CONF_LINE_COMMENT

/** CONF_DOC_COMMENT */
public class ConfWidget {
    public int confField = 1;

    public String confPublic() {
        String a = "CONF_BODY_A";
        return a;
    }

    /** CONF_PRIVATE_DOC */
    private String confPrivate() {
        return File.separator;
    }

    public String confOther() {
        String b = "CONF_BODY_B";
        return b;
    }

    public String confThrows() throws java.io.IOException {
        String c = "CONF_BODY_C";
        return c;
    }
}
