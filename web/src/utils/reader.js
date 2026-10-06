export function readerPath(row, order = 1, fallbackKind = '') {
  const kind = row?.kind || fallbackKind
  const comicId = row?.comicId
  if (!kind || !comicId) return null
  return `/reader/${kind}/${encodeURIComponent(comicId)}/${order}`
}

// Resume is resolved inside the reader, so opening a new tab stays synchronous.
export function openReaderWindow(row, order = 1, fallbackKind = '', resume = true) {
  const path = readerPath(row, order, fallbackKind)
  if (!path) return
  const url = window.location.pathname + '#' + path + (resume ? '?resume=1' : '?page=1')
  const newTab = localStorage.getItem('ms-reader-new-tab') !== '0' && !window.matchMedia('(max-width: 768px)').matches
  if (newTab) window.open(url, '_blank', 'noopener')
  else window.location.hash = path + (resume ? '?resume=1' : '?page=1')
}
export default readerPath
