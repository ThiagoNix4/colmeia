/** Distribui dados dos PTYs para o card certo (um listener IPC só). */
type Handler = (data: string) => void
type ExitHandler = (code: number) => void

const dataHandlers = new Map<string, Handler>()
const exitHandlers = new Map<string, ExitHandler>()

window.canvas.pty.onData((id, data) => dataHandlers.get(id)?.(data))
window.canvas.pty.onExit((id, code) => exitHandlers.get(id)?.(code))

export function subscribe(id: string, onData: Handler, onExit: ExitHandler) {
  dataHandlers.set(id, onData)
  exitHandlers.set(id, onExit)
  return () => {
    if (dataHandlers.get(id) === onData) dataHandlers.delete(id)
    if (exitHandlers.get(id) === onExit) exitHandlers.delete(id)
  }
}
