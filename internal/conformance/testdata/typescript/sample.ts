import { sep } from "path";

// CONF_LINE_COMMENT

/** CONF_DOC_COMMENT */
export class ConfWidget {
    confField: number = 1;
    #confSecret = 1;

    confPublic(): string {
        const a = "CONF_BODY_A";
        return a;
    }

    /** CONF_PRIVATE_DOC */
    private confPrivate(): string {
        return sep;
    }

    #confHashMethod() { return 1; }

    confOther(): string {
        const b = "CONF_BODY_B";
        return b;
    }
}

export function confTopLevel(n: number): number { return n; }

export const confConst: number = 1;

export const confArrow = (n: number) => n;

export function confIsNum(x: unknown): x is number { return true; }

export type ConfId = string;

export enum ConfColor { ConfRed, ConfGreen = "green" }

export enum ConfFlags { ConfNone = 0, ConfShift = 1 << 2, ConfNeg = -1 }

export namespace ConfSpace {
    export function confInSpace(): number { return 1; }
}
declare module "conf-ambient" {
    export function confAmbient(): void;
    function confAmbientImplicit(): void;
}
declare namespace ConfDeclared {
    function confDeclaredFn(): void;
}
