// Per-user, per-tab navigation state. Storage may be unavailable in private mode.
import { auth } from '@/store/auth'
function key(name) { return `ms-view:${auth.user?.username || ''}:${name}` }
export function readViewState(name) {
  try { return JSON.parse(sessionStorage.getItem(key(name)) || '{}') || {} } catch (_) { return {} }
}
export function writeViewState(name, value) {
  try { sessionStorage.setItem(key(name), JSON.stringify(value)) } catch (_) {}
}
