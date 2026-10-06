// Each reader owns one queue. A generation isolates late callbacks after a chapter switch.
export function createPreloader(makeImage = () => new Image(), limit = 2) {
  let generation = 0, queue = [], active = new Map(), complete = new Set()
  let urlFor, onSize
  function pump() {
    while (active.size < limit && queue.length) {
      const page = queue.shift()
      if (active.has(page) || complete.has(page)) continue
      const token = generation, image = makeImage()
      active.set(page, image)
      const finish = (ok) => {
        if (token !== generation) return
        image.onload = image.onerror = null
        active.delete(page)
        if (ok) {
          complete.add(page)
          onSize(page, image.naturalWidth, image.naturalHeight)
        }
        pump()
      }
      image.onload = () => finish(true)
      image.onerror = () => finish(false)
      image.src = urlFor(page)
    }
  }
  return {
    reset() {
      generation++
      queue = []
      for (const image of active.values()) {
        image.onload = image.onerror = null
        image.src = ''
      }
      active.clear()
      complete.clear()
    },
    schedule(pages, url, size) {
      urlFor = url
      onSize = size
      const wanted = new Set(pages)
      for (const [p, image] of active) {
        if (!wanted.has(p)) {
          image.onload = image.onerror = null
          image.src = ''
          active.delete(p)
        }
      }
      queue = [...wanted]
      pump()
    }
  }
}
