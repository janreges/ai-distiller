import { sep } from "node:path";
import ConfReact, { confHook as confUseHook } from "conf-react";

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

export function confWithHook(hook = confUseHook) {
    return ConfReact.createElement(hook);
}

export * from "./conf-all";
export { confPicked as ConfRenamed } from "./conf-picked";
export { confArrow };
export default ConfWidget;
export async function* confGen(n) {}
export const confExported = 3;
export const confWrapped = confWrap((confP) => {
    return "CONF_WRAPPED_BODY";
});
export const confAsyncArrow = async (confU) => confU;
