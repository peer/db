import type { Router } from "@/types"

import { createApp } from "vue"
import { createRouter, createWebHistory } from "vue-router"
import Main from "@/Main.vue"
import { routes } from "@/../routes.json"
import "./main.css"
import siteContext from "@/context"

// During development when requests are proxied to Vite, placeholders
// in HTML files are not rendered. So we set them here as well.
document.title = siteContext.site.title

const router = createRouter({
  history: createWebHistory(),
  scrollBehavior(to, from, savedPosition) {
    // DocumentSearch route handles its own scrolling through "at" query parameter.
    if (to.name === "DocumentSearch") {
      return false
    }
    if (savedPosition) {
      return savedPosition
    } else {
      return { top: 0 }
    }
  },
  routes: routes
    .filter((route) => route.get)
    .map((route) => ({
      path: route.path,
      name: route.name,
      component: () => import(`./views/${route.name}.vue`),
      props: true,
    })),
}) as Router

const apiRouter = createRouter({
  history: createWebHistory(),
  routes: routes
    .filter((route) => route.api)
    .map((route) => ({
      path: `/api${route.path}`,
      name: route.name,
      component: () => null,
      props: true,
    })),
})

router.apiResolve = apiRouter.resolve.bind(apiRouter)

createApp(Main).use(router).mount("main")
