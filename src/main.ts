import { createApp } from "vue"
import { createRouter, createWebHistory } from "vue-router"
import Main from "@/Main.vue"
import { routes } from "@/../routes.json"
import "./main.css"

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
  routes: routes.map((route) => ({
    path: route.path,
    name: route.name,
    component: () => import(`./views/${route.name}.vue`),
    props: true,
  })),
})

createApp(Main).use(router).mount("main")
