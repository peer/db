import { createApp } from "vue"
import { createRouter, createWebHistory } from "vue-router"
import Main from "@/Main.vue"
import { routes } from "@/../routes.json"
import "./main.css"

const router = createRouter({
  history: createWebHistory(),
  routes: routes.map((route) => ({
    path: route.path,
    name: route.name,
    component: () => import(`./views/${route.name}.vue`),
    props: true,
  })),
})

createApp(Main).use(router).mount("main")
