<script setup lang="ts">
import type { D } from "@/document"
import type { RefCheckState, RefFilterTreeNode } from "@/types"

import { ArrowTopRightOnSquareIcon } from "@heroicons/vue/20/solid"
import { computed } from "vue"
import { useI18n } from "vue-i18n"
import { RouterLink } from "vue-router"

import CheckBox from "@/components/CheckBox.vue"
import WithDocument from "@/components/WithDocument.vue"
import DisplayLabel from "@/partials/DisplayLabel.vue"
import { useLocked } from "@/progress"
import { DIRECT_REF_FILTER_PREFIX, loadingWidth, MISSING_VALUE_ID, VALUES_NOT_SHOWN_PREFIX } from "@/utils"

const props = defineProps<{
  node: RefFilterTreeNode
  propsKey: string
  checkStates: ReadonlyMap<string, RefCheckState>
  onToggle: (node: RefFilterTreeNode) => void
}>()

const locked = useLocked()
const { t } = useI18n({ useScope: "global" })

// The tri-state for this row's value, computed once for the whole panel.
// Checked covers a value selected on its own, a value covered by a selected ancestor,
// and a value all of whose children are selected; indeterminate covers a value
// with only part of its subtree selected.
const state = computed(() => props.checkStates.get(props.node.res.id) ?? { checked: false, indeterminate: false })
const checked = computed(() => state.value.checked)
const indeterminate = computed(() => state.value.indeterminate)

function handleToggle() {
  props.onToggle(props.node)
}

const WithDocumentD = WithDocument<D>

const inputId = computed(() => "ref/" + props.propsKey + "/" + props.node.key)
</script>

<template>
  <li>
    <div class="flex items-baseline gap-x-1">
      <!--
        A "values not shown" marker is non-interactive: it has no checkbox and is not a selectable value. It
        marks a parent whose children were truncated by the server cap and shows the document gap in parens.
      -->
      <template v-if="node.res.id.startsWith(VALUES_NOT_SHOWN_PREFIX)">
        <span class="text-gray-600"
          ><i>{{ t("common.status.valuesNotShownShort") }}</i> ({{ node.res.count }})</span
        >
      </template>
      <template v-else>
        <CheckBox :id="inputId" :model-value="checked" :indeterminate="indeterminate" @update:model-value="handleToggle" />
        <template v-if="node.res.id === MISSING_VALUE_ID">
          <label :for="inputId" :class="locked ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
            ><i>{{ t("common.values.missing") }}</i></label
          >
          <label :for="inputId" :class="locked ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'">({{ node.res.count }})</label>
        </template>
        <template v-else-if="node.res.id.startsWith(DIRECT_REF_FILTER_PREFIX)">
          <label :for="inputId" :class="locked ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
            ><i>{{ t("common.values.direct") }}</i></label
          >
          <label :for="inputId" :class="locked ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'">({{ node.res.count }})</label>
        </template>
        <template v-else>
          <WithDocumentD :id="node.res.id" name="DocumentGet">
            <template #default="{ doc, url }">
              <label :for="inputId" :class="locked ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'" :data-url="url"><DisplayLabel :doc="doc" /></label>
            </template>
            <template #loading="{ url }">
              <div
                class="pd-withdocument-loading h-2 rounded-sm bg-slate-200 motion-safe:animate-pulse"
                :data-url="url"
                :class="[loadingWidth(node.res.id)]"
                aria-hidden="true"
              ></div>
            </template>
          </WithDocumentD>
          <label :for="inputId" :class="locked ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'">({{ node.res.count }})</label>
          <!--
          tabindex="-1" keeps the open-link icon out of the keyboard tab
          order so Tab jumps between filters without stopping
          on each row's icon. Mouse users can still click it.
        -->
          <RouterLink :to="{ name: 'DocumentGet', params: { id: node.res.id } }" class="link" tabindex="-1"
            ><ArrowTopRightOnSquareIcon :alt="t('common.icons.link')" class="inline size-5 align-text-top"
          /></RouterLink>
        </template>
      </template>
    </div>
    <ul v-if="node.children.length > 0" class="pl-6">
      <RefFilterTreeRow v-for="child in node.children" :key="child.key" :node="child" :props-key="propsKey" :check-states="checkStates" :on-toggle="onToggle" />
    </ul>
  </li>
</template>
