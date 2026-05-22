<script setup lang="ts">
import type { D } from "@/document"
import type { RefFilterTreeNode } from "@/types"

import { ArrowTopRightOnSquareIcon } from "@heroicons/vue/20/solid"
import { computed } from "vue"
import { useI18n } from "vue-i18n"
import { RouterLink } from "vue-router"

import CheckBox from "@/components/CheckBox.vue"
import WithDocument from "@/components/WithDocument.vue"
import DisplayLabel from "@/partials/DisplayLabel.vue"
import { useLocked } from "@/progress"
import { loadingWidth } from "@/utils"

const props = defineProps<{
  node: RefFilterTreeNode
  propsKey: string
  selectedSet: ReadonlySet<string>
  onToggle: (node: RefFilterTreeNode) => void
}>()

const locked = useLocked()
const { t } = useI18n({ useScope: "global" })

// All res.id values in the rendered subtree (this node plus its descendants). The
// cascade triggered by clicking this checkbox covers exactly this set, so what the
// user sees underneath the checkbox is what gets toggled.
function collectSubtreeIds(n: RefFilterTreeNode, out: Set<string>): Set<string> {
  out.add(n.res.id)
  for (const c of n.children) {
    collectSubtreeIds(c, out)
  }
  return out
}

const subtreeIds = computed(() => Array.from(collectSubtreeIds(props.node, new Set<string>())))

const aggregateChecked = computed(() => subtreeIds.value.every((id) => props.selectedSet.has(id)))

const anyDescendantSelected = computed(() => subtreeIds.value.some((id) => props.selectedSet.has(id)))

// Visual third state: not fully checked, but at least one id in the subtree is selected.
const indeterminate = computed(() => !aggregateChecked.value && anyDescendantSelected.value)

function handleToggle() {
  props.onToggle(props.node)
}

const WithDocumentD = WithDocument<D>

const inputId = computed(() => "ref/" + props.propsKey + "/" + props.node.key)
</script>

<template>
  <li>
    <div class="flex items-baseline gap-x-1">
      <CheckBox :id="inputId" :model-value="aggregateChecked" :indeterminate="indeterminate" @update:model-value="handleToggle" />
      <template v-if="node.res.id === '__MISSING__'">
        <label :for="inputId" :class="locked ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
          ><i>{{ t("common.values.missing") }}</i></label
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
    </div>
    <ul v-if="node.children.length > 0" class="pl-6">
      <RefFilterTreeRow v-for="child in node.children" :key="child.key" :node="child" :props-key="propsKey" :selected-set="selectedSet" :on-toggle="onToggle" />
    </ul>
  </li>
</template>
