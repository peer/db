<script setup lang="ts">
import { onBeforeUnmount, onMounted, useTemplateRef, watch } from "vue"
import { useI18n } from "vue-i18n"
import { useRoute, useRouter } from "vue-router"

import { useFixedNavbar } from "@/navbar"
import { useVisibilityTracking } from "@/visibility"

const { t } = useI18n({ useScope: "global" })
const route = useRoute()
const router = useRouter()
// In fixed-navbar mode the navbar always occupies the top of the viewport, so scroll targets must always sit below it.
const fixedNavbar = useFixedNavbar()
// Use the full viewport so the first heading is reported as visible when the user is at the top of the page.
const { track, visibles } = useVisibilityTracking({ rootMargin: "0px", threshold: 0 })

const props = defineProps<{
  // Each id must correspond to an element in the page (typically a heading or section).
  targets: { id: string; label: string }[]
}>()

const tocRef = useTemplateRef<HTMLElement>("tocRef")
// Tracks the elements we set view-timeline-* on, so cleanup can find them even after props.targets changes.
let timelineTargets: HTMLElement[] = []
// Ids we have handed to useVisibilityTracking, so we can untrack them on cleanup or props change.
const trackedIds = new Set<string>()
// Holds the exact hash we just set via router.replace from a visibility-driven update, so the route.hash
// watcher can tell that change apart from user-initiated ones (click, back/forward, paste).
let suppressedHash: string | null = null

function timelineName(id: string): string {
  return `--toc-${id}`
}

function setupTimelines() {
  if (!tocRef.value) return
  const sidebar = tocRef.value
  const sidebarStyle = getComputedStyle(sidebar)
  const sidebarStickyTop = parseFloat(sidebarStyle.top) || 0
  const sidebarHeight = sidebar.getBoundingClientRect().height
  const paddingTop = parseFloat(sidebarStyle.paddingTop) || 0
  const paddingBottom = parseFloat(sidebarStyle.paddingBottom) || 0
  const sidebarInnerHeight = sidebarHeight - paddingTop - paddingBottom

  const items = sidebar.querySelectorAll<HTMLElement>(".pd-toc-item")
  const count = props.targets.length
  if (count === 0 || items.length === 0) return

  timelineTargets = []
  const lastItem = items[items.length - 1]
  // Use paddingTop instead of firstItem.offsetTop so anything in the slot above the items (e.g. a heading) is included in contentHeight.
  const contentHeight = lastItem.offsetTop + lastItem.offsetHeight - paddingTop
  // Distance to translate each item from its natural (top-stack) position down to its bottom-stack position.
  const bottomOffset = Math.max(0, sidebarInnerHeight - contentHeight)
  sidebar.style.setProperty("--pd-toc-bottom-offset", `${bottomOffset}px`)

  // Length-based animation end (in pixels of cover scroll). Same for every item - animation runs for bottomOffset
  // pixels (or bottomOffset + |navbar_top| when navbar is hidden, via calc with --pd-navbar-top in CSS).
  sidebar.style.setProperty("--pd-toc-lockstep-end-base", `${bottomOffset}px`)

  const vh = window.innerHeight
  const names: string[] = []
  for (let i = 0; i < count; i++) {
    const entry = props.targets[i]
    const item = items[i]
    if (!item) continue
    const el = document.getElementById(entry.id)
    if (!el) continue
    const topStackY = sidebarStickyTop + item.offsetTop
    const bottomStackY = topStackY + bottomOffset
    const name = timelineName(entry.id)
    el.style.setProperty("view-timeline-name", name)
    el.style.setProperty("view-timeline-axis", "block")
    // Only the top inset shifts by --pd-navbar-top. Bottom stays static so the scrollport grows when navbar hides,
    // giving the cover range room to span bottomOffset + |navbar_top| of scroll. Slope stays 1 -> lockstep exact.
    el.style.setProperty("view-timeline-inset", `calc(${topStackY}px + var(--pd-navbar-top, 0px)) ${vh - bottomStackY}px`)
    timelineTargets.push(el)
    names.push(name)
    track(entry.id)(el)
    trackedIds.add(entry.id)
  }
  // Hoist named timelines to body so TOC items in a different subtree can reference them via animation-timeline.
  document.body.style.setProperty("timeline-scope", names.join(", "))
  // Gate the animation on a class so items default to top-stack until setup is fully wired up.
  sidebar.classList.add("pd-toc-active")
}

function cleanupTimelines() {
  tocRef.value?.classList.remove("pd-toc-active")
  for (const el of timelineTargets) {
    el.style.removeProperty("view-timeline-name")
    el.style.removeProperty("view-timeline-axis")
    el.style.removeProperty("view-timeline-inset")
  }
  timelineTargets = []
  for (const id of trackedIds) {
    track(id)(null)
  }
  trackedIds.clear()
  document.body.style.removeProperty("timeline-scope")
  tocRef.value?.style.removeProperty("--pd-toc-bottom-offset")
  tocRef.value?.style.removeProperty("--pd-toc-lockstep-end-base")
  tocRef.value?.style.removeProperty("--pd-toc-release-start-base")
  tocRef.value?.style.removeProperty("--pd-toc-release-end")
  tocRef.value?.style.removeProperty("--pd-toc-release-amount-base")
}

// Compute the page-scroll range over which sticky releases at the bottom, and the amount it releases by.
// A scroll-driven animation in CSS uses these to translate the whole nav back into place - runs on the compositor.
// Base values assume the navbar is visible; the navbar-hidden case is handled in CSS via var(--pd-navbar-top),
// since the sidebar height (and therefore release start/amount) varies with the navbar offset.
function updateReleaseRange() {
  if (!tocRef.value) return
  const sidebar = tocRef.value
  const parent = sidebar.parentElement
  if (!parent) return
  const vh = window.innerHeight
  const parentDocBottom = parent.getBoundingClientRect().bottom + window.scrollY
  const docHeight = document.documentElement.scrollHeight
  // Equivalent to (parentDocBottom - stickyTop - sidebarHeight) when navbar visible.
  const baseReleaseStart = Math.max(0, parentDocBottom - vh)
  const releaseEnd = Math.max(baseReleaseStart, docHeight - vh)
  const baseReleaseAmount = releaseEnd - baseReleaseStart
  sidebar.style.setProperty("--pd-toc-release-start-base", `${baseReleaseStart}px`)
  sidebar.style.setProperty("--pd-toc-release-end", `${releaseEnd}px`)
  sidebar.style.setProperty("--pd-toc-release-amount-base", `${baseReleaseAmount}px`)
}

// Coalesce repeated resize events into one update per frame.
// Resize re-runs setupTimelines without cleanup: timeline names never disappear, so animation-timeline references stay linked.
let resizeRaf: number | null = null

function onResize() {
  if (resizeRaf !== null) return
  resizeRaf = requestAnimationFrame(() => {
    resizeRaf = null
    setupTimelines()
    updateReleaseRange()
  })
}

// Content keeps loading after mount (labels, lists, images), moving the targets and
// changing the parent bottom and document height the release range is computed from.
// A stale release range makes the compensation animation run before sticky actually
// releases, visibly drifting the whole nav down mid-page until the true page end. So
// re-measure whenever the parent or the body resizes, not only on window resizes.
// Both animations run on transform, which does not affect layout, so re-measuring
// cannot retrigger the observer in a loop.
let resizeObserver: ResizeObserver | null = null

onMounted(() => {
  setupTimelines()
  updateReleaseRange()
  window.addEventListener("resize", onResize, { passive: true })
  resizeObserver = new ResizeObserver(onResize)
  resizeObserver.observe(document.body)
  if (tocRef.value?.parentElement) {
    resizeObserver.observe(tocRef.value.parentElement)
  }
  // Initial-load scroll: if the URL has a matching hash, scroll to it. The route.hash watcher below uses
  // immediate: false so it does not race with router/route resolution on first navigation; this block covers
  // that case. requestAnimationFrame defers one frame so layout is fully settled before measuring.
  const id = route.hash.slice(1)
  if (id && props.targets.some((target) => target.id === id)) {
    requestAnimationFrame(() => scrollToId(id))
  }
})

onBeforeUnmount(() => {
  window.removeEventListener("resize", onResize)
  resizeObserver?.disconnect()
  resizeObserver = null
  if (resizeRaf !== null) cancelAnimationFrame(resizeRaf)
  cleanupTimelines()
})

watch(
  () => props.targets,
  () => {
    cleanupTimelines()
    setupTimelines()
  },
  // flush: "post" so the new links are in the DOM by the time setupTimelines queries them.
  // deep: true to also fire on in-place mutation of the targets array.
  { flush: "post", deep: true },
)

// Watch the topmost visible heading and reflect it in the URL via router.replace (no history pollution).
// Iterates props.targets in order; the first one in the visibles set is the topmost in DOM order.
watch(
  () => {
    for (const target of props.targets) {
      if (visibles.value.has(target.id)) return target.id
    }
    return null
  },
  async (topId) => {
    if (topId === null) return
    const newHash = `#${topId}`
    if (route.hash === newHash) return
    suppressedHash = newHash
    await router.replace({ hash: newHash })
  },
)

function scrollToId(id: string) {
  const el = document.getElementById(id)
  if (!el) return
  const stickyTop = tocRef.value ? parseFloat(getComputedStyle(tocRef.value).top) || 0 : 0
  // getBoundingClientRect() returns the border box; add margin-top explicitly as breathing room below the navbar.
  const marginTop = parseFloat(getComputedStyle(el).marginTop) || 0
  const elDocY = window.scrollY + el.getBoundingClientRect().top
  // In auto-hide mode, anticipate navbar state at the destination: scrolling up reveals the navbar (heading
  // lands at navbar bottom), scrolling down hides it (heading lands at viewport top). In fixed mode the navbar
  // never hides, so we must always offset for it - otherwise the heading would land underneath it.
  const targetVisible = elDocY - stickyTop - marginTop
  const targetY = fixedNavbar.value || targetVisible < window.scrollY ? targetVisible : elDocY - marginTop
  window.scrollTo({ top: targetY, behavior: "smooth" })
}

async function onItemClick(event: MouseEvent, id: string) {
  // Let the browser handle modified clicks (open in new tab/window, save link, etc.).
  if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || event.button !== 0) return
  event.preventDefault()
  // The route.hash watcher does the actual scrolling - pushing a new hash makes back/forward symmetric.
  await router.push({ hash: `#${id}` })
}

// Watch route.hash so click, back/forward, and external hash changes (URL paste) all scroll the same way.
// Global router scrollBehavior returns false for hash navigation, leaving the scroll to this watcher.
// Initial-load is handled in onMounted instead of via immediate: true to avoid racing with route resolution.
watch(
  () => route.hash,
  (newHash) => {
    // Skip observer-driven replaces: we updated the URL because the user already scrolled there.
    const wasSuppressed = suppressedHash !== null && newHash === suppressedHash
    suppressedHash = null
    if (wasSuppressed) return
    const id = newHash.slice(1)
    if (!id) return
    // Only react to hashes that match one of our targets - leave unrelated fragments alone.
    if (!props.targets.some((target) => target.id === id)) return
    scrollToId(id)
  },
  { flush: "post" },
)
</script>

<template>
  <nav ref="tocRef" :aria-label="t('partials.TableOfContents.title')" class="pd-toc sticky top-[var(--pd-navbar-height)] flex flex-col gap-y-1 py-4">
    <slot />
    <a
      v-for="target in targets"
      :key="target.id"
      :href="`#${target.id}`"
      :aria-current="route.hash === `#${target.id}` ? 'location' : undefined"
      :style="{ animationTimeline: timelineName(target.id) }"
      class="pd-toc-item link block shrink-0 py-1 text-left text-sm"
      @click="onItemClick($event, target.id)"
    >
      {{ target.label }}
    </a>
  </nav>
</template>
