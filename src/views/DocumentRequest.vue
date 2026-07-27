<!--
The page where a signed-in user can request access to a document: an access they do not have yet,
either because they cannot read it at all (e.g. a document shared with them through a link) or because
they can read it but want more (e.g. updating it).

Requesting access records a HAS_REQUESTED_PERMISSION claim (with the requested action, the user, and the
note) on the document, which the users with the permissions action can then approve or deny on the
permissions tab of the document edit page.
-->

<script setup lang="ts">
import type { D } from "@/document"

import { computed, onBeforeUnmount, ref, useId, useTemplateRef } from "vue"
import { useI18n } from "vue-i18n"
import { useRouter } from "vue-router"

import { deleteFromCache, FetchError, getURL, postJSON } from "@/api"
import { hasDocumentPermission, isSignedIn } from "@/auth"
import siteContext from "@/context"
import Button from "@/components/Button.vue"
import ButtonLink from "@/components/ButtonLink.vue"
import TextArea from "@/components/TextArea.vue"
import Footer from "@/partials/Footer.vue"
import InputBadges from "@/partials/InputBadges.vue"
import NavBar from "@/partials/NavBar.vue"
import PermissionActionsInput from "@/partials/PermissionActionsInput.vue"
import SearchResult from "@/partials/SearchResult.vue"
import { permissionActions, permissionActionsClosure } from "@/permissions"
import { useBusy } from "@/progress"
import { encodeQuery } from "@/utils"
import { focusFirstInvalid, useValidationRegistry } from "@/validation"

const props = defineProps<{
  id: string
}>()

const { t } = useI18n({ useScope: "global" })
const router = useRouter()

const busy = useBusy()

// A site without document-level permissions has no permissions to request: the backend does not serve
// the request API there either.
const available = computed(() => !siteContext.features.disableDocumentPermissions)
const requested = ref(false)
const requestError = ref(false)

// The form's validation registry: the actions input and the note register themselves with it, so the
// Request button follows whether anything has been filled in and submitting validates both.
const { validateAll, anyError, allEmpty, inputs } = useValidationRegistry()

// The permissions being requested and the optional note for whoever decides the request.
const selected = ref<string[]>([])
const note = ref("")

// The label elements naming the two inputs. They are spans and not <label for=...> elements, like the
// labels of the fields form (see FieldsFormField): a label of a composite input has no single control
// to point at, and a native one would also hand its hover state to the input it names.
const permissionLabelId = useId()
const noteLabelId = useId()

// The two inputs, for their labels to focus.
const permissionActionsRef = useTemplateRef<{ inputEl: () => HTMLElement | null }>("permissionActionsRef")
const noteRef = useTemplateRef<{ inputEl: () => HTMLElement | null }>("noteRef")

// Clicking a label focuses its input, the way a <label for=...> would. mousedown with the default
// prevented (the @mousedown.prevent in the template) sends the focus straight there instead of
// blurring to the body first.
function onPermissionLabelMousedown(): void {
  permissionActionsRef.value?.inputEl()?.focus()
}

function onNoteLabelMousedown(): void {
  noteRef.value?.inputEl()?.focus()
}

// The document, when the caller can read it. It is what the page can show of the document, and it
// decides which permissions are worth asking for: those the caller does not hold on it already. A
// caller who cannot read it (the common case for requesting the read permission) is offered all of
// them, because nothing about the document is known.
const doc = ref<D | null>(null)

const availableActions = computed(() =>
  permissionActions.filter((action) => doc.value === null || !hasDocumentPermission(action.id, doc.value)).map((action) => action.id),
)

async function loadDocument() {
  busy.value += 1
  try {
    const { doc: fetched } = await getURL<D>(
      router.apiResolve({
        name: "DocumentGet",
        params: {
          id: props.id,
        },
      }).href,
      null,
      abortController.signal,
      busy,
    )
    if (abortController.signal.aborted) {
      return
    }
    doc.value = fetched
  } catch (err) {
    if (abortController.signal.aborted) {
      return
    }
    if (err instanceof FetchError && err.status === 403) {
      // The caller cannot read the document, an expected state on this page, not a failure.
      return
    }
    // TODO: Show notification with error.
    console.error("DocumentRequest.loadDocument", err)
  } finally {
    busy.value -= 1
  }
}

const abortController = new AbortController()
onBeforeUnmount(() => {
  abortController.abort()
})

// eslint-disable-next-line @typescript-eslint/no-floating-promises
loadDocument()

async function onRequest() {
  if (abortController.signal.aborted) {
    return
  }

  // The button is enabled as soon as anything has been filled in, which can be the note alone, so the
  // form is validated here: an incomplete one surfaces its error and gets the focus instead of being
  // sent.
  await validateAll(abortController.signal, { final: true })
  if (abortController.signal.aborted) {
    return
  }
  if (anyError.value) {
    focusFirstInvalid(inputs)
    return
  }

  requestError.value = false
  busy.value += 1
  try {
    await postJSON(
      router.apiResolve({
        name: "DocumentRequest",
        params: {
          id: props.id,
        },
      }).href,
      { actions: [...permissionActionsClosure(selected.value)], note: note.value },
      abortController.signal,
      busy,
    )
    if (abortController.signal.aborted) {
      return
    }
    // The request is recorded on the document, so its cached response is dropped: the document page
    // (which the user typically returns to) then shows the request instead of the state before it.
    deleteFromCache(
      router.apiResolve({
        name: "DocumentGet",
        params: {
          id: props.id,
        },
      }).href,
    )
    requested.value = true
  } catch (err) {
    if (abortController.signal.aborted) {
      return
    }
    console.error("DocumentRequest.onRequest", err)
    requestError.value = true
  } finally {
    busy.value -= 1
  }
}
</script>

<template>
  <Teleport to="header">
    <NavBar />
  </Teleport>
  <div class="pd-documentrequest mt-[var(--pd-navbar-offset)] flex w-full flex-col gap-y-1 border-t border-transparent p-1 sm:gap-y-4 sm:p-4">
    <div class="flex flex-col gap-y-4 rounded-sm border border-gray-200 bg-white p-4 shadow-sm">
      <template v-if="available && isSignedIn()">
        <div>
          <h1 class="text-3xl font-bold drop-shadow-xs">{{ t("views.DocumentRequest.title") }}</h1>
          <p v-if="requested" class="mt-1 text-gray-700">{{ t("views.DocumentRequest.requested") }}</p>
          <p v-else class="mt-1 text-gray-700">{{ t("views.DocumentRequest.confirm") }}</p>
        </div>
        <!--
          The document itself, when the caller can read it, so they see what they are asking about.
          Once the request is recorded the confirmation stands alone, like the rest of the page.
        -->
        <SearchResult v-if="doc && !requested" :result="{ id }" flat />
        <template v-if="!requested">
          <div v-if="availableActions.length === 0" class="text-gray-700">{{ t("views.DocumentRequest.nothingToRequest") }}</div>
          <!--
            The two fields are laid out like the fields of the fields form: the label with its badges on
            the left (above the field below the md breakpoint), the field itself on the right, and the
            field's hints under it.
          -->
          <div v-else class="grid grid-cols-1 gap-y-4 md:grid-cols-[20%_1fr] md:items-start md:gap-x-3">
            <div class="flex flex-row flex-wrap items-center gap-1 font-medium text-gray-700 md:flex-col md:items-start">
              <span :id="permissionLabelId" class="cursor-pointer pt-0.5 leading-none" @mousedown.prevent="onPermissionLabelMousedown">{{
                t("views.DocumentRequest.permission")
              }}</span>
              <div class="flex flex-row flex-wrap gap-1">
                <InputBadges required multiple hide-changed />
              </div>
            </div>
            <PermissionActionsInput ref="permissionActionsRef" v-model="selected" :labelledby="permissionLabelId" :available="availableActions" />
            <div class="flex flex-row flex-wrap items-center gap-1 font-medium text-gray-700 md:flex-col md:items-start">
              <span :id="noteLabelId" class="cursor-pointer pt-0.5 leading-none" @mousedown.prevent="onNoteLabelMousedown">{{ t("views.DocumentRequest.note") }}</span>
            </div>
            <div class="flex flex-col">
              <TextArea
                id="documentrequest-note"
                ref="noteRef"
                v-model="note"
                :aria-labelledby="noteLabelId"
                aria-describedby="documentrequest-note-hint"
                class="w-full"
              />
              <p id="documentrequest-note-hint" class="mt-1 text-sm text-neutral-500 italic">{{ t("views.DocumentRequest.noteHint") }}</p>
            </div>
          </div>
        </template>
        <div v-if="requestError" class="text-error-600">{{ t("common.errors.unexpected") }}</div>
        <!--
          Leaving the page goes to the document, which is offered only to a caller who can read it: the
          cancel of an unsent request, and after it was sent the way to its permissions, where it is now
          listed. The empty first cell keeps the primary action on the right when there is no cancel.
        -->
        <div class="flex flex-row justify-between gap-4">
          <div>
            <ButtonLink v-if="doc && !requested" id="documentrequest-button-cancel" :to="{ name: 'DocumentGet', params: { id } }">{{
              t("common.buttons.cancel")
            }}</ButtonLink>
          </div>
          <div>
            <Button
              v-if="!requested && availableActions.length > 0"
              id="documentrequest-button-request"
              type="button"
              primary
              :disabled="allEmpty"
              :progress="busy"
              @click.prevent="onRequest"
              >{{ t("views.DocumentRequest.request") }}</Button
            >
            <ButtonLink
              v-if="requested && doc"
              id="documentrequest-button-permissions"
              primary
              :to="{ name: 'DocumentGet', params: { id }, query: encodeQuery({ tab: 'permissions' }) }"
              >{{ t("common.buttons.permissions") }}</ButtonLink
            >
          </div>
        </div>
      </template>
      <div v-else-if="!available" class="my-1 text-center sm:my-4">{{ t("views.DocumentRequest.notAvailable") }}</div>
      <div v-else class="my-1 text-center sm:my-4">{{ t("views.DocumentRequest.notSignedIn") }}</div>
    </div>
  </div>
  <Teleport to="footer">
    <Footer class="border-t border-slate-50 bg-slate-200 shadow-sm" />
  </Teleport>
</template>
