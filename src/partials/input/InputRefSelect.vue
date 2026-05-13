<!--
InputRefSelect provides a radio-button-list picker for a document reference.

It loads up to 100 search results once and renders them as a fieldset of
radio buttons. Unlike InputRef which uses a typeahead combobox, this
component is suited for short option lists where the user should see all
choices at once.

The legend is supplied via the default slot.

We do not use :read-only or :disabled pseudo classes to style the component because
we want component to retain how it visually looks even if DOM element's read-only or
disabled attributes are set, unless they are set through component's props.
This is used during transitions/animations to disable the component by directly setting
its DOM attributes without flickering how the component looks.
-->

<script setup lang="ts">
import type { D } from "@/document"
import type { Result } from "@/types"

import { ArrowTopRightOnSquareIcon } from "@heroicons/vue/20/solid"
import { computed, onBeforeMount, onBeforeUnmount, ref, useId } from "vue"
import { useI18n } from "vue-i18n"
import { useRouter } from "vue-router"

import { postJSON } from "@/api"
import RadioButton from "@/components/RadioButton.vue"
import WithDocument from "@/components/WithDocument.vue"
import DisplayLabel from "@/partials/DisplayLabel.vue"
import { useLocked, useProgress } from "@/progress"
import { loadingWidth } from "@/utils"

const props = withDefaults(
  defineProps<{
    readonly?: boolean
  }>(),
  {
    readonly: false,
  },
)

const model = defineModel<string>({ default: "" })

const baseId = useId()

const { t } = useI18n({ useScope: "global" })
const router = useRouter()

// Data loading only, no controls.
const progress = useProgress()

const locked = useLocked()
const inactive = computed(() => locked.value || props.readonly)

const abortController = new AbortController()
const dataLoading = ref(true)
const dataLoadingError = ref("")
const searchResults = ref<Result[]>([])

onBeforeUnmount(() => {
  abortController.abort()
})

onBeforeMount(async () => {
  progress.value += 1
  try {
    const response = await postJSON<Result[]>(
      router.apiResolve({ name: "SearchJustResults" }).href,
      {
        query: "",
      },
      abortController.signal,
      progress,
    )
    if (abortController.signal.aborted) {
      return
    }

    // We use only the first 100 results.
    searchResults.value = response.slice(0, 100)
  } catch (err) {
    if (abortController.signal.aborted) {
      return
    }
    console.error("InputRefSelect.onBeforeMount", err)
    // eslint-disable-next-line @typescript-eslint/restrict-template-expressions
    dataLoadingError.value = `${err}`
  } finally {
    dataLoading.value = false
    progress.value -= 1
  }
})

const WithPeerDBDocument = WithDocument<D>
</script>

<template>
  <fieldset class="pd-inputrefselect" :aria-busy="dataLoading || undefined">
    <legend class="mb-1"><slot /></legend>
    <div class="grid grid-cols-[max-content_auto] gap-x-1 gap-y-0.5">
      <template v-if="dataLoading">
        <template v-for="i in 3" :key="i">
          <div class="mx-2 my-1.5 h-2 w-4 rounded-sm bg-slate-200 motion-safe:animate-pulse" aria-hidden="true" />
          <div class="my-1.5 h-2 rounded-sm bg-slate-200 motion-safe:animate-pulse" :class="[loadingWidth(`${baseId}-placeholder-${i}`)]" aria-hidden="true" />
        </template>
      </template>
      <div v-else-if="dataLoadingError" class="col-span-2 p-2 text-error-600">{{ t("common.errors.unexpected") }}</div>
      <template v-else>
        <template v-for="result in searchResults" :key="result.id">
          <RadioButton :id="`${baseId}-${result.id}`" v-model="model" :name="baseId" :value="result.id" :disabled="props.readonly" class="mx-2" />
          <div class="flex items-baseline gap-x-1">
            <WithPeerDBDocument :id="result.id" name="DocumentGet">
              <template #default="{ doc, url }">
                <label :for="`${baseId}-${result.id}`" :class="inactive ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'" :data-url="url"
                  ><DisplayLabel :doc="doc"
                /></label>
              </template>
              <template #loading="{ url }">
                <span
                  class="pd-withdocument-loading h-2 rounded-sm bg-slate-200 motion-safe:animate-pulse"
                  :data-url="url"
                  :class="[loadingWidth(result.id)]"
                  aria-hidden="true"
                />
              </template>
            </WithPeerDBDocument>
            <!--
              tabindex="-1" keeps the open-link icon out of the keyboard tab
              order so Tab jumps between form fields without stopping on each
              row's icon. Mouse users can still click it; the icon is here as
              a "view document" affordance, not a primary action.
            -->
            <RouterLink :to="{ name: 'DocumentGet', params: { id: result.id } }" class="link" tabindex="-1"
              ><ArrowTopRightOnSquareIcon :alt="t('common.icons.link')" class="inline size-5 align-text-top"
            /></RouterLink>
          </div>
        </template>
        <div v-if="searchResults.length === 0" class="col-span-2 p-2"
          ><i>{{ t("partials.input.InputRefSelect.noOptions") }}</i></div
        >
      </template>
    </div>
  </fieldset>
</template>
