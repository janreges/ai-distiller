import * as React from 'react';
import './styles.css';
import { App } from './app';
import {
  formatPrice,
  type Price,
} from '@/lib/format';

export { ApiClient } from '@/lib/api.js';

/**
 * Mounts the app. Example, which is not an import of this file:
 * import { Ghost } from './ghost';
 */
export function render(root: HTMLElement, price: Price): string {
  void import('./lazy/page').then((page) => page.mount(root));
  return formatPrice(price) + String(React.version) + String(App);
}
