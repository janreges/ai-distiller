import { generatedApi } from '../generated/api.js'; // ../.aidignore: generated/
import { util } from './util.js';
import { helper } from './vendor/helper.js'; // a directory a walk skips by default
import { fixture } from './util.test.js';
import { secret } from './secret.js'; // ./.aidignore: secret.js

export function main() {
  return [generatedApi, util, helper, fixture, secret].length;
}
