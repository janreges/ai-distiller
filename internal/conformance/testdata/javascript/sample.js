import { sep } from "node:path";
import ConfReact, { confHook as confUseHook } from "conf-react";
import React from "react";

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
export let confLetArrow = (confL) => confL;
export var confVarValue = 3;
export const confFnExpr = function (confQ) {};
export const confGenExpr = function* () {};
export class ConfGenerators {
    *confGenMethod() {}
    async *confAsyncGenMethod() {}
}
/** @type {Map<string, Set<number>>} */
export const confTypedMap = new Map();
export function ConfJsxView() {
    return <div className="conf" />;
}
