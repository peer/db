import { describe, expect, test } from "vitest"

import { Queue } from "@/queue"

// Deferred is a manually resolvable promise so a test can hold tasks "in flight" and release them on demand.
function deferred(): { promise: Promise<void>; resolve: () => void } {
  let resolve!: () => void
  const promise = new Promise<void>((r) => {
    resolve = r
  })
  return { promise, resolve }
}

describe("Queue", () => {
  test("never runs more than concurrency tasks at once", async () => {
    const concurrency = 3
    const total = 20
    const queue = new Queue({ concurrency })

    let inFlight = 0
    let maxInFlight = 0
    const gates = Array.from({ length: total }, () => deferred())

    const results = gates.map((gate, i) =>
      queue.add(async () => {
        inFlight++
        maxInFlight = Math.max(maxInFlight, inFlight)
        await gate.promise
        inFlight--
        return i
      }),
    )

    // Let the queue start as many tasks as it will. With a correct queue only "concurrency" tasks start.
    await Promise.resolve()
    await Promise.resolve()
    expect(maxInFlight).toBeLessThanOrEqual(concurrency)
    expect(inFlight).toBe(concurrency)

    // Release tasks one by one and confirm the cap holds as queued tasks take over.
    for (const gate of gates) {
      gate.resolve()
      await Promise.resolve()
      await Promise.resolve()
      expect(inFlight).toBeLessThanOrEqual(concurrency)
    }

    await Promise.all(results)
    expect(maxInFlight).toBe(concurrency)
  })
})
