import { sep } from "node:path";

// CONF_LINE_COMMENT

/** CONF_DOC_COMMENT */
export class ConfWidget {
    confPublic() {
        const a = "CONF_BODY_A";
        return a;
    }

    /** CONF_PRIVATE_DOC */
    #confPrivate() {
        return sep;
    }

    _confUnderscore() {
        return 1;
    }

    #confHashMethod() {
        return 1;
    }

    confOther() {
        const b = "CONF_BODY_B";
        return b;
    }
}

const confArrow = (a) => a + 1;
