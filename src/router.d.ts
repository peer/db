import type { RouteLocationRaw, RouteLocationNormalizedLoaded, RouteLocation } from "vue-router"

declare module "vue-router" {
  interface Router {
    apiResolve(
      to: RouteLocationRaw,
      currentLocation?: RouteLocationNormalizedLoaded,
    ): RouteLocation & {
      href: string
    }
  }
}
