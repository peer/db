export class Queue {
  concurrency = 1

  _tasks: (() => Promise<void>)[] = []
  _pendingCount = 0

  constructor(options?: { concurrency?: number }) {
    if (options?.concurrency) {
      this.concurrency = options.concurrency
    }
  }

  add<T>(fn: () => Promise<T>, options?: { signal?: AbortSignal }): Promise<T> {
    return new Promise<T>((resolve, reject) => {
      const run = async (): Promise<void> => {
        this._pendingCount++
        try {
          options?.signal?.throwIfAborted()

          const result = await fn()
          resolve(result)
        } catch (error: unknown) {
          reject(error)
        } finally {
          this._pendingCount--
          this._flush()
        }
      }

      this._tasks.push(run)
      this._flush()
    })
  }

  _flush(): void {
    while (this._pendingCount < this.concurrency) {
      const task = this._tasks.shift()
      if (!task) {
        return
      }

      task()
    }
  }
}
