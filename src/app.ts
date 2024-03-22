import { createApp, ref } from "vue"
import { createRouter, createWebHistory } from "vue-router"
import App from "@/App.vue"
import { progressKey } from "./progress"
import { routes } from "@/../routes.json"
import "./app.css"
import siteContext from "@/context"

// During development when requests are proxied to Vite, placeholders
// in HTML files are not rendered. So we set them here as well.
document.title = siteContext.title

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
      strict: true,
    })),
})

const apiRouter = createRouter({
  history: createWebHistory(),
  routes: routes
    .filter((route) => route.api)
    .map((route) => ({
      path: route.path === "/" ? "/api" : `/api${route.path}`,
      name: route.name,
      component: () => null,
      props: true,
      strict: true,
    })),
})

router.apiResolve = apiRouter.resolve.bind(apiRouter)

createApp(App).use(router).provide(progressKey, ref(0)).mount("main")
