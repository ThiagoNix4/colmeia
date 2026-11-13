import { contextBridge, ipcRenderer, type IpcRendererEvent } from 'electron'
import type { CanvasEvent, LinkEdge, CardInfo, NoteCard, Preset, SpawnRequest, UsageReport, UsageSettings, Workspace } from './types'

const on = <A extends unknown[]>(channel: string, cb: (...args: A) => void) => {
  const fn = (_e: IpcRendererEvent, ...args: unknown[]) => cb(...(args as A))
  ipcRenderer.on(channel, fn)
  return () => {
    ipcRenderer.removeListener(channel, fn)
  }
}

const api = {
  pty: {
    ensure: (req: SpawnRequest): Promise<{ replay: string; info: CardInfo }> => ipcRenderer.invoke('pty:ensure', req),
    restart: (id: string): Promise<void> => ipcRenderer.invoke('pty:restart', id),
    write: (id: string, data: string) => ipcRenderer.send('pty:write', id, data),
    resize: (id: string, cols: number, rows: number) => ipcRenderer.send('pty:resize', id, cols, rows),
    kill: (id: string) => ipcRenderer.send('pty:kill', id),
    detachAll: () => ipcRenderer.send('pty:detachAll'),
    killAll: () => ipcRenderer.send('pty:killAll'),
    setMeta: (id: string, patch: { title?: string }) => ipcRenderer.send('pty:setMeta', id, patch),
    send: (ids: string[], text: string): Promise<string[]> => ipcRenderer.invoke('pty:send', ids, text),
    list: (): Promise<CardInfo[]> => ipcRenderer.invoke('pty:list'),
    onData: (cb: (id: string, data: string) => void) => on('pty:data', cb),
    onExit: (cb: (id: string, code: number) => void) => on('pty:exit', cb)
  },
  app: {
    info: (): Promise<{ socketPath: string; presetsPath: string; home: string }> => ipcRenderer.invoke('app:info'),
    newId: (): Promise<string> => ipcRenderer.invoke('app:newId'),
    pickDir: (current?: string): Promise<string | null> => ipcRenderer.invoke('dialog:pickDir', current),
    onCanvasEvent: (cb: (ev: CanvasEvent) => void) => on('canvas:event', cb),
    /** O main pede um flush do workspace antes de fechar. Responder com flushed(). */
    onFlush: (cb: () => void) => on('app:flush', cb),
    flushed: () => ipcRenderer.send('app:flushed')
  },
  presets: {
    get: (): Promise<Preset[]> => ipcRenderer.invoke('presets:get'),
    /** Recebe só os agentes do usuário; devolve a lista completa (builtins primeiro). */
    save: (list: Preset[]): Promise<Preset[]> => ipcRenderer.invoke('presets:save', list)
  },
  ws: {
    list: (): Promise<string[]> => ipcRenderer.invoke('ws:list'),
    load: (name?: string): Promise<Workspace> => ipcRenderer.invoke('ws:load', name),
    save: (ws: Workspace) => ipcRenderer.send('ws:save', ws),
    remove: (name: string): Promise<void> => ipcRenderer.invoke('ws:delete', name),
    rename: (from: string, to: string): Promise<string> => ipcRenderer.invoke('ws:rename', from, to)
  },
  links: {
    set: (edges: LinkEdge[]) => ipcRenderer.send('links:set', edges)
  },
  notes: {
    list: (ws: string): Promise<NoteCard[]> => ipcRenderer.invoke('notes:list', ws),
    create: (ws: string, title: string): Promise<NoteCard> => ipcRenderer.invoke('notes:create', ws, title),
    update: (ws: string, id: string, patch: Partial<Omit<NoteCard, 'id' | 'workspace'>>): Promise<NoteCard> =>
      ipcRenderer.invoke('notes:update', ws, id, patch),
    remove: (ws: string, id: string, deleteFile?: boolean): Promise<void> =>
      ipcRenderer.invoke('notes:remove', ws, id, deleteFile),
    read: (ws: string, ref: string): Promise<{ body: string; mtime: number }> => ipcRenderer.invoke('notes:read', ws, ref),
    write: (ws: string, ref: string, text: string, baseMtime: number): Promise<{ mtime: number }> =>
      ipcRenderer.invoke('notes:write', ws, ref, text, baseMtime),
    root: (): Promise<string> => ipcRenderer.invoke('notes:root')
  },
  usage: {
    get: (): Promise<UsageReport> => ipcRenderer.invoke('usage:get'),
    setSettings: (s: UsageSettings): Promise<UsageReport> => ipcRenderer.invoke('usage:setSettings', s),
    onChange: (cb: (r: UsageReport) => void) => on('usage:changed', cb)
  }
}

contextBridge.exposeInMainWorld('canvas', api)
export type CanvasApi = typeof api
