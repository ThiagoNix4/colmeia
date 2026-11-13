import type { CanvasApi } from '../electron/preload'
declare global {
  interface Window {
    canvas: CanvasApi
  }
}
export {}
