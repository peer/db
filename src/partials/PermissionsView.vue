<!--
The permissions tab of the document page: the users the document grants access to, with the actions
each of them holds, and the access requests waiting for a decision, which are listed only when there
are any. It is a read-only rendering of the document's own permission claims, which the "all
properties" tab shows as claims like any other; changing them happens in the permissions tab of the
document edit page (see PermissionsForm), which the buttons under the two lists lead to.

A caller holding the permissions action gets a button per list: editing the granted access, and
deciding the pending requests. Everybody else gets, at the end, the way to ask for access themselves,
which requires being signed in (a request records who made it). A request of the signed-in user
carries a button to withdraw it, the one change to the document this page makes on its own.

Access which role grants provide is not listed here: role grants are not part of the document.
-->

<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { ClaimTypes } from "@/document"

import { computed, onBeforeUnmount } from "vue"
import { useI18n } from "vue-i18n"
import { useRouter } from "vue-router"

import { deleteFromCache, postJSON } from "@/api"
import { currentIdentityId, isSignedIn } from "@/auth"
import Button from "@/components/Button.vue"
import ButtonLink from "@/components/ButtonLink.vue"
import { useDocumentActions } from "@/document-actions"
import IdentityInline from "@/partials/IdentityInline.vue"
import { permissionActionLabel, permissionActionOrder, permissionGrants, permissionRequests } from "@/permissions"
import { useBusy } from "@/progress"
import { delay } from "@/utils"

const props = defineProps<{
  // The document the permissions belong to, which the access request page is opened for.
  id: string
  claims: DeepReadonly<ClaimTypes>
}>()

// Withdrawing a request changes the document, so the page is asked to load it again.
const emit = defineEmits<{ cancelled: [] }>()

const { t } = useI18n({ useScope: "global" })
const router = useRouter()

const busy = useBusy()

// What the document page offers on the document: whether the caller may change its permissions, and
// the edit session its buttons start, with the progress it runs under (see DocumentActions). Outside
// of that page there is none of it, and then there is nothing to offer but asking for access.
const actions = useDocumentActions()
const canUpdatePermissions = computed(() => actions?.canUpdatePermissions.value ?? false)
const editBusy = computed(() => actions?.editBusy.value ?? 0)

// Editing the granted access and deciding the pending requests both happen on the permissions tab of
// the edit page, so both buttons start an edit session on it.
async function onEdit(): Promise<void> {
  await actions?.edit("permissions")
}

// How long to wait for the withdrawal to be committed before the document is loaded again.
// TODO: Remove once we use websocket to watch for new changes.
const commitDelay = 1000 // In milliseconds.

const abortController = new AbortController()
onBeforeUnmount(() => {
  abortController.abort()
})

// Which claims grant an action is of no interest here, so only the actions are kept, ordered the way
// the permissions tab of the edit page lists them, so that every user's access reads the same way.
const users = computed(() =>
  permissionGrants(props.claims).map((grant) => ({
    user: grant.user,
    actions: [...grant.actions.keys()].sort((a, b) => permissionActionOrder(a) - permissionActionOrder(b)),
  })),
)

const requests = computed(() => permissionRequests(props.claims))

// actionLabel is the label of an action, falling back to its identifier for an action which the
// permission actions list does not cover.
function actionLabel(action: string): string {
  return permissionActionLabel(action, t) ?? action
}

// A request can be withdrawn by the user who made it, so only their own requests offer it.
function isOwnRequest(user: string): boolean {
  return isSignedIn() && user === currentIdentityId.value
}

// onCancel withdraws the request for the action, which the backend does together with the requests
// for the actions requiring it, and asks the page for the document again so the lists match it. The
// backend answers the same way whether or not there was anything to withdraw, so there is nothing to
// tell the user apart from the reloaded lists.
async function onCancel(action: string) {
  if (abortController.signal.aborted) {
    return
  }

  busy.value += 1
  try {
    await postJSON(
      router.apiResolve({
        name: "DocumentDeleteRequest",
        params: {
          id: props.id,
        },
      }).href,
      { action },
      abortController.signal,
      busy,
    )
    if (abortController.signal.aborted) {
      return
    }
    // The document changed, so its cached response is dropped before the page is asked for it again.
    deleteFromCache(
      router.apiResolve({
        name: "DocumentGet",
        params: {
          id: props.id,
        },
      }).href,
    )
    // The session recording the withdrawal is committed asynchronously, so the document is asked for
    // once the commit has had the time to land.
    // TODO: Use websocket to watch for new changes.
    await delay(commitDelay, abortController.signal)
    if (abortController.signal.aborted) {
      return
    }
    emit("cancelled")
  } catch (err) {
    if (abortController.signal.aborted) {
      return
    }
    // TODO: Show notification with error.
    console.error("PermissionsView.onCancel", err)
  } finally {
    busy.value -= 1
  }
}
</script>

<template>
  <div class="pd-permissionsview flex flex-col gap-y-4">
    <div>
      <h2 class="text-xl font-bold">{{ t("partials.PermissionsView.usersTitle") }}</h2>
      <p v-if="users.length === 0" class="mt-1 text-gray-700">{{ t("partials.PermissionsView.noUsers") }}</p>
      <ul v-else class="mt-2 flex flex-col gap-y-3">
        <li v-for="row of users" :key="row.user" class="flex flex-col gap-y-2 rounded border border-slate-300 p-3">
          <IdentityInline :subject="row.user" class="font-medium" />
          <ul class="flex flex-row flex-wrap content-start items-baseline gap-1 text-sm">
            <li v-for="action of row.actions" :key="action" class="rounded-xs bg-slate-100 px-1.5 py-0.5 leading-none text-gray-600 shadow-xs">{{
              actionLabel(action)
            }}</li>
          </ul>
        </li>
      </ul>
      <div v-if="canUpdatePermissions" class="mt-4 flex flex-row justify-end">
        <Button :progress="editBusy" type="button" @click.prevent="onEdit">{{ t("common.buttons.editPermissions") }}</Button>
      </div>
    </div>
    <div v-if="requests.length > 0">
      <h2 class="text-xl font-bold">{{ t("partials.PermissionsView.requestsTitle") }}</h2>
      <ul class="mt-2 flex flex-col gap-y-3">
        <!-- One claim can ask on behalf of several users, so it takes both to tell its requests apart. -->
        <li v-for="request of requests" :key="`${request.claimID}-${request.user}`" class="flex flex-col gap-y-2 rounded border border-slate-300 p-3">
          <div class="flex flex-row items-center justify-between gap-4">
            <IdentityInline :subject="request.user" class="font-medium" />
            <!-- Only the user who made the request can withdraw it. -->
            <Button v-if="isOwnRequest(request.user)" type="button" :progress="busy" @click.prevent="onCancel(request.action)">{{ t("common.buttons.cancel") }}</Button>
          </div>
          <div class="text-gray-700">{{ t("partials.PermissionsView.requestedAction") }}: {{ actionLabel(request.action) }}</div>
          <div v-if="request.note" class="break-words whitespace-pre-wrap text-gray-700">{{ request.note }}</div>
        </li>
      </ul>
      <div v-if="canUpdatePermissions" class="mt-4 flex flex-row justify-end">
        <Button :progress="editBusy" type="button" @click.prevent="onEdit">{{ t("common.buttons.manageRequests") }}</Button>
      </div>
    </div>
    <div v-if="!canUpdatePermissions && isSignedIn()" class="flex flex-row justify-end">
      <ButtonLink :to="{ name: 'DocumentRequest', params: { id } }">{{ t("common.buttons.requestPermissions") }}</ButtonLink>
    </div>
  </div>
</template>
