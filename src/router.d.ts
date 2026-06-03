import type { RouteLocation, RouteLocationNormalizedLoaded, RouteLocationRaw, _RouterClassic } from "vue-router"

declare module "vue-router" {
  interface TypesConfig {
    Router: _RouterClassic & {
      apiResolve(
        to: RouteLocationRaw,
        currentLocation?: RouteLocationNormalizedLoaded,
      ): RouteLocation & {
        href: string
      }
    }
  }

  interface RouteMeta {
    // True if this SPA route has a corresponding Vue view. False for routes
    // that are registered for URL-building purposes only and are served
    // directly by the backend.
    hasView?: boolean
  }
}
