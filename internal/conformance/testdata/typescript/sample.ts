import { sep } from "path";
import ConfDefault, { confNamed as ConfAlias } from "./conf-mod";
import type { ConfOnlyType } from "./conf-types";
import * as confNs from "./conf-ns";
import { confGone as ConfGoneAlias } from "./conf-gone";
import ConfAliasUsed = confNs.ConfInner;
import ConfAliasGone = confNs.ConfUnused;
import type { $ConfDollar } from "./conf-dollar";
import { confObs$ } from "./conf-obs";
import { ConfAbove } from "./conf-above";
import * as confGeo from "./conf-geo";
import ConfPoly = confGeo.ConfPoly;

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

export function confImports(a: ConfDefault, b: ConfAlias, c: confNs.T): ConfOnlyType { return a; }

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

export * from "./conf-all";
export * as confAllNs from "./conf-all-ns";
export { confPicked, confOrig as ConfRenamed } from "./conf-picked";
export type { ConfReType } from "./conf-retype";
const confListed = 2;
export { confListed };
export default confListed;

export interface ConfBased extends ConfGenericBase<string>, confNs.ConfQualified<number> {}
export class ConfDerived extends confNs.ConfBaseClass<{ id: number }> implements ConfBased {}

export function confParams({ confA, confB = 1 }: ConfOpts, [confX]: [number], confOpt?: string, confDef: number = 0, ...confRest: string[]): void {}
export class ConfProps {
    constructor(readonly confRo: number, private confHid: string) {}
}
export class ConfAccessors<T extends object = {}> {
    get confSize(): number { return 0; }
    set confSize(v: number) {}
    async *confIter(): AsyncGenerator<T> {}
}
export function* confGen(): Generator<number> {}
export const confWrapped = confWrap<string>((confP: string) => {
    return "CONF_WRAPPED_BODY";
});
export const confGeneric = async <T,>(confV: T): Promise<T> => confV;
export const confTypedArrow: ConfHandler = (confReq) => confReq;
export function confMultiType(confM: {
    confK: string;
}): void {}
enum ConfListedEnum { ConfL }
export { ConfListedEnum };
export let confLetArrow = (confL: number) => confL;
export var confVarValue = 3;
export interface ConfMarkers {
    readonly confRoId: string;
    confOptEmail?: string;
    [confKey: string]: unknown;
    confOptRun?(confA: number): void;
}
export class ConfMarked {
    [confK: string]: any;
    confOptName?: string;
}
export const confAliased: ConfAliasUsed = 1;
export function confDollar(x: $ConfDollar): typeof confObs$ { return confObs$; }
export function confAbove(x: ConfAbove, p: ConfPoly): void {}
export namespace ConfLate {
    import ConfLateAlias = confNs.ConfLateInner;
    export const confLate: ConfLateAlias = 1;
}
export as namespace ConfLib;
