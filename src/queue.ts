export class Queue {
  concurrency = 1

  _tasks: (() => void)[] = []
  _pendingCount = 0

  constructor(options?: { concurrency?: number }) {
    if (options?.concurrency) {
      this.concurrency = options.concurrency
    }
  }

  add<T>(fn: () => Promise<T>, options?: { signal?: AbortSignal }): Promise<T> {
    return new Promise<T>((resolve, reject) => {
      const run = () => {
        if (options?.signal?.aborted) {
          //eslint-disable-next-line @typescript-eslint/prefer-promise-reject-errors
          reject(options.signal.reason)
          // An aborted task never counted towards _pendingCount, so just try to start the next one.
          this._flush()
          return
        }

        this._pendingCount++
        // The task stays counted until fn settles, so _pendingCount reflects
        // in-flight work and the concurrency limit holds.
        fn()
          .then(resolve, reject)
          .finally(() => {
            this._pendingCount--
            this._flush()
          })
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
