import { sep } from "path";

// CONF_LINE_COMMENT

/** CONF_DOC_COMMENT */
export class ConfWidget {
    confField: number = 1;

    confPublic(): string {
        const a = "CONF_BODY_A";
        return a;
    }

    confOther(): string {
        const b = "CONF_BODY_B";
        return b;
    }

    /** CONF_PRIVATE_DOC */
    private confPrivate(): string {
        return sep;
    }
}
