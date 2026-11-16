export type Price = number;

export function formatPrice(price: Price): string {
  return price.toFixed(2);
}
