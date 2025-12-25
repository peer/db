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
        } else {
          this._pendingCount++
          try {
            fn().then(resolve, reject)
          } finally {
            this._pendingCount--
          }
        }
        this._flush()
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
