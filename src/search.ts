import type { Ref } from "vue"
import type { Router } from "vue-router"

export async function doSearch(router: Router, progress: Ref<boolean>, form: HTMLFormElement) {
  progress.value = true
  try {
    const response = await fetch(
      router.resolve({
        name: "DocumentSearch",
      }).href,
      {
        method: "POST",
        headers: {
          Accept: "application/json",
          "Content-Type": "application/x-www-form-urlencoded; charset=UTF-8",
        },
        // Have to cast to "any". See: https://github.com/microsoft/TypeScript/issues/30584
        body: new URLSearchParams(new FormData(form) as any),
        mode: "same-origin",
        credentials: "omit",
        redirect: "error",
        referrer: document.location.href,
        referrerPolicy: "strict-origin-when-cross-origin",
      },
    )
    if (!response.ok) {
      throw new Error(`fetch error ${response.status}: ${await response.text()}`)
    }
    router.push({
      name: "DocumentSearch",
      query: await response.json(),
    })
  } finally {
    progress.value = false
  }
}
