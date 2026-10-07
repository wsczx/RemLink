// guacamole-common-js 1.5 的最小类型声明（仅覆盖本项目用到的 API）
declare module 'guacamole-common-js' {
  namespace Guacamole {
    class Status {
      constructor(code: number, message?: string)
      code: number
      message?: string
      isError(): boolean
    }
    namespace Status {
      const Code: Record<string, number>
    }

    class Tunnel {
      static readonly INTERNAL_DATA_OPCODE: string
      state: number
      uuid: string | null
      receiveTimeout: number
      unstableThreshold: number
      connect(data?: string): void
      disconnect(): void
      sendMessage(...elements: any[]): void
      isConnected(): boolean
      onerror: ((status: Status) => void) | null
      onstatechange: ((state: number) => void) | null
      oninstruction: ((opcode: string, args: string[]) => void) | null
    }
    namespace Tunnel {
      // CONNECTING 0 / OPEN 1 / CLOSED 2 / UNSTABLE 3
      const State: { CONNECTING: 0; OPEN: 1; CLOSED: 2; UNSTABLE: 3 }
    }
    class WebSocketTunnel extends Tunnel {
      constructor(url: string)
    }

    class OutputStream {
      index: number
      onack: ((status: Status) => void) | null
      sendBlob(data: string): void
      sendEnd(): void
    }
    class InputStream {
      index: number
      onblob: ((data: string) => void) | null
      onend: (() => void) | null
      sendAck(message: string, code: number): void
    }
    class StringWriter {
      constructor(stream: OutputStream)
      sendText(text: string): void
      sendEnd(): void
      onack: ((status: Status) => void) | null
    }
    class StringReader {
      constructor(stream: InputStream)
      ontext: ((text: string) => void) | null
      onend: (() => void) | null
    }

    class Layer {
      width: number
      height: number
    }
    class Display {
      getElement(): HTMLDivElement
      getWidth(): number
      getHeight(): number
      getScale(): number
      scale(scale: number): void
      showCursor(shown: boolean): void
      getDefaultLayer(): Layer
      flatten(): HTMLCanvasElement
      onresize: ((width: number, height: number) => void) | null
      oncursor: ((canvas: HTMLCanvasElement, x: number, y: number) => void) | null
    }

    class Client {
      constructor(tunnel: Tunnel)
      connect(data?: string): void
      disconnect(): void
      getDisplay(): Display
      sendSize(width: number, height: number): void
      sendKeyEvent(pressed: 0 | 1 | boolean, keysym: number): void
      sendMouseState(state: Mouse.State, applyDisplayScale?: boolean): void
      createClipboardStream(mimetype: string): OutputStream
      createArgumentValueStream(mimetype: string, name: string): OutputStream
      onstatechange: ((state: number) => void) | null
      onerror: ((status: Status) => void) | null
      onclipboard: ((stream: InputStream, mimetype: string) => void) | null
      onrequired: ((parameters: string[]) => void) | null
      onname: ((name: string) => void) | null
    }
    namespace Client {
      // IDLE 0 / CONNECTING 1 / WAITING 2 / CONNECTED 3 / DISCONNECTING 4 / DISCONNECTED 5
      const State: { IDLE: 0; CONNECTING: 1; WAITING: 2; CONNECTED: 3; DISCONNECTING: 4; DISCONNECTED: 5 }
    }

    // Guacamole.Mouse.Event.Target 的结构（为避免类与命名空间的循环继承，单独声明）
    class MouseEventTarget {
      currentState: Mouse.State
      onEach(types: string[], listener: (e: Mouse.Event) => void): void
      press(button: string): void
      release(button: string): void
      click(button: string): void
      reset(): void
    }
    class Mouse extends MouseEventTarget {
      constructor(element: Element)
      scrollThreshold: number
      touchMouseThreshold: number
      setCursor(canvas: HTMLCanvasElement, x: number, y: number): boolean
    }
    namespace Mouse {
      class State {
        constructor(template?: Partial<State>)
        x: number
        y: number
        left: boolean
        middle: boolean
        right: boolean
        up: boolean
        down: boolean
      }
      class Event {
        type: string
        state: State
        preventDefault(): void
        stopPropagation(): void
      }
      class Touchscreen extends MouseEventTarget {
        constructor(element: Element)
        scrollThreshold: number
      }
      class Touchpad extends MouseEventTarget {
        constructor(element: Element)
        scrollThreshold: number
      }
    }

    class Keyboard {
      constructor(element?: Element | Document)
      onkeydown: ((keysym: number) => boolean | void) | null
      onkeyup: ((keysym: number) => void) | null
      listenTo(element: Element | Document): void
      press(keysym: number): boolean
      release(keysym: number): void
      reset(): void
    }

    class AudioPlayer {
      static getSupportedTypes(): string[]
    }
    class VideoPlayer {
      static getSupportedTypes(): string[]
    }

    class SessionRecording {
      constructor(source: Blob | Tunnel)
      connect(data?: string): void
      disconnect(): void
      abort(): void
      getDisplay(): Display
      isPlaying(): boolean
      getPosition(): number
      getDuration(): number
      play(): void
      pause(): void
      cancel(): void
      seek(position: number, callback?: () => void): void
      onload: (() => void) | null
      onerror: ((message: string) => void) | null
      onabort: (() => void) | null
      onprogress: ((duration: number, parsedSize: number) => void) | null
      onplay: (() => void) | null
      onpause: (() => void) | null
      onseek: ((position: number, current: number, total: number) => void) | null
    }
  }
  export default Guacamole
}
