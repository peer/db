// esm-seedrandom package is missing types.
// See: https://github.com/shanewholloway/js-esm-seedrandom/issues/1

declare module "esm-seedrandom" {
  export interface PRNG {
    (): number
    quick(): number
    int32(): number
    double(): number
  }

  export function prng_alea(seed?: string): PRNG
}
