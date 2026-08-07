<script setup lang="ts">
import { computed, ref } from "vue"
import { useI18n } from "vue-i18n"
import { useRoute, useRouter } from "vue-router"

import { siteRoles } from "@/auth"
import Button from "@/components/Button.vue"
import CheckBox from "@/components/CheckBox.vue"
import Footer from "@/partials/Footer.vue"
import NavBar from "@/partials/NavBar.vue"
import { useBusy } from "@/progress"
import { encodeQuery, redirectServerSide } from "@/utils"

const { t } = useI18n({ useScope: "global" })
const route = useRoute()
const router = useRouter()

const busy = useBusy()

// The roles the site declares, which are the ones a mock user can be signed in as, and those chosen.
const roles = siteRoles()
const selected = ref<string[]>([])

// The flow the mock sign-in began, which the callback matches its own record against: without it there
// is no sign-in to finish, which happens when the page is opened on its own instead of being arrived at.
const state = computed((): string => {
  const value = route.query.state
  return (Array.isArray(value) ? value[0] : value) ?? ""
})

function onToggle(role: string, checked: boolean) {
  if (checked) {
    selected.value = [...selected.value, role]
  } else {
    selected.value = selected.value.filter((other) => other !== role)
  }
}

// Signing in hands the chosen roles to the callback as the code, the way an issuer hands over a code
// standing for whom the user signed in as (see mockCode in auth/mock.go). The callback then sets the
// session cookie and sends the browser where the sign-in started, so it is a server-side navigation.
function onSignIn() {
  const code = `mock:${[...selected.value].sort().join(",")}`
  const target = router.resolve({ name: "AuthCallback", query: encodeQuery({ code, state: state.value }) }).href
  redirectServerSide(target, false, busy)
}
</script>

<template>
  <Teleport to="header">
    <NavBar />
  </Teleport>
  <div class="pd-authmocksignin mt-[var(--pd-navbar-offset)] flex w-full flex-col p-1 sm:p-4 xl:px-16">
    <div class="flex flex-col gap-y-4 rounded-sm border border-gray-200 bg-white p-4 shadow-sm">
      <div>
        <h1 class="text-3xl font-bold drop-shadow-xs">{{ t("views.AuthMockSignIn.title") }}</h1>
        <p class="mt-1 text-gray-700">{{ t("views.AuthMockSignIn.description") }}</p>
      </div>
      <form v-if="state" class="flex flex-col gap-y-4" @submit.prevent="onSignIn">
        <!-- The roles are shown as the site names them, because that is what the sign-in claims. -->
        <div v-if="roles.length" class="flex flex-col gap-y-1">
          <label v-for="role of roles" :key="role" class="flex cursor-pointer items-center gap-x-2">
            <CheckBox :model-value="selected.includes(role)" @update:model-value="onToggle(role, $event as boolean)" />
            {{ role }}
          </label>
        </div>
        <p v-else class="text-gray-700">{{ t("views.AuthMockSignIn.noRoles") }}</p>
        <div class="flex flex-row justify-end">
          <Button id="authmocksignin-button-signin" type="submit" primary :progress="busy">{{ t("common.buttons.signIn") }}</Button>
        </div>
      </form>
      <p v-else class="text-error-600">{{ t("views.AuthMockSignIn.noState") }}</p>
    </div>
  </div>
  <Teleport to="footer">
    <Footer class="border-t border-slate-50 bg-slate-200 shadow-sm" />
  </Teleport>
</template>
