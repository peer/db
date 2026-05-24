import type { Component } from "vue"

import { createApp, ref } from "vue"
import { createRouter, createWebHistory } from "vue-router"

import "@/app.css"
import App from "@/App.vue"
import RouterLink from "@/components/RouterLink.vue"
import { configKey } from "@/config"
import siteContext from "@/context"
import i18n from "@/i18n"
import { progressKey, rootProgressKey } from "@/progress"
import routes from "@/routes"
import twMerge from "@/tw-merge"

// During development when requests are proxied to Vite, placeholders
// in HTML files are not rendered. So we set them here as well.
if (siteContext.title) {
  document.title = siteContext.title
}

const VIEW_PATH_REGEX = /\/views\/(.+)\.vue$/

// Enumerate Vue views known to Vite at build time. Only routes whose name
// matches one of these views become SPA-routed by useInternalLinksClick.
// The rest (e.g. /f/:id which is served directly by the backend as a binary
// download) are left to the browser. We use ./views/ and not @/views/ here
// because with @/views/ Vite does not resolve any views.
const viewModules = import.meta.glob<{ default: Component }>("./views/*.vue")
const viewLoaders = new Map<string, () => Promise<{ default: Component }>>()
for (const [path, loader] of Object.entries(viewModules)) {
  const match = VIEW_PATH_REGEX.exec(path)
  if (match) {
    viewLoaders.set(match[1], loader)
  }
}

const router = createRouter({
  history: createWebHistory(),
  scrollBehavior(to, from, savedPosition) {
    // Search route handles its own scrolling through "at" query parameter.
    if (to.name === "SearchGet") {
      return false
    }
    // Hash navigation is handled by component-level watchers (e.g. TableOfContents) so
    // they can apply navbar-aware offsets and smooth scrolling.
    if (to.hash) {
      return false
    }
    if (savedPosition) {
      return savedPosition
    } else {
      return { top: 0 }
    }
  },
  routes: Object.entries(routes)
    .filter(([, route]) => route.handlers)
    .map(([name, route]) => {
      const loader = viewLoaders.get(name)
      return {
        path: route.path,
        name,
        // Routes without a matching view (e.g. /f/:id served directly by the
        // backend) are still registered so name-based URL building works, but
        // are flagged via meta so useInternalLinksClick can skip them.
        component: loader ?? (() => null),
        props: true,
        strict: true,
        meta: { hasView: loader !== undefined },
      }
    }),
})

const apiRouter = createRouter({
  history: createWebHistory(),
  routes: Object.entries(routes)
    .filter(([, route]) => route.api)
    .map(([name, route]) => ({
      path: route.path === "/" ? "/api" : `/api${route.path}`,
      name,
      component: () => null,
      props: true,
      strict: true,
    })),
})

router.apiResolve = apiRouter.resolve.bind(apiRouter)

const rootProgress = ref(0)

const app = createApp(App).use(router)

// We replace Vue Router's RouterLink with ours.
delete app._context.components["RouterLink"]
app.component("RouterLink", RouterLink)

app
  .use(i18n)
  .use(twMerge)
  .provide(progressKey, rootProgress)
  .provide(rootProgressKey, rootProgress)
  .provide(
    configKey,
    ref({
      fixedNavbar: false,
    }),
  )
  .mount("main")
