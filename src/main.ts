import { createApp } from "vue"
import { createRouter, createWebHistory } from "vue-router"
import App from "@/App.vue"
import { routes } from "@/../routes.json"
import "./main.scss"

const router = createRouter({
  history: createWebHistory(),
  routes: routes.map((route) => ({
    path: route.path,
    name: route.name,
    component: () => import(`./views/${route.name}.vue`),
    props: true,
  })),
})

createApp(App).use(router).mount("#app")
