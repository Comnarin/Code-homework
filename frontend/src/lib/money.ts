const thb = new Intl.NumberFormat("th-TH", {
  style: "currency",
  currency: "THB",
});

/** Formats an integer amount of satang (1/100 THB) as baht, for example "฿228.00". */
export function formatTHB(satang: number): string {
  return thb.format(satang / 100);
}
