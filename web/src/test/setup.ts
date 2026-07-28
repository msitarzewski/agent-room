import "@testing-library/jest-dom/vitest";
import { afterEach, beforeEach, vi } from "vitest";
import { cleanup } from "@testing-library/react";

class TestWebSocket {
  static readonly CONNECTING = 0;
  static readonly OPEN = 1;
  static readonly CLOSING = 2;
  static readonly CLOSED = 3;
  readonly CONNECTING = 0;
  readonly OPEN = 1;
  readonly CLOSING = 2;
  readonly CLOSED = 3;
  readonly url: string;
  readonly protocol = "";
  readonly extensions = "";
  readonly bufferedAmount = 0;
  readonly binaryType: BinaryType = "blob";
  readyState = TestWebSocket.CONNECTING;
  onopen: ((this: WebSocket, ev: Event) => unknown) | null = null;
  onclose: ((this: WebSocket, ev: CloseEvent) => unknown) | null = null;
  onerror: ((this: WebSocket, ev: Event) => unknown) | null = null;
  onmessage: ((this: WebSocket, ev: MessageEvent) => unknown) | null = null;
  private listeners = new Map<string, Set<EventListener>>();
  static instances: TestWebSocket[] = [];

  constructor(url: string | URL) {
    this.url = String(url);
    TestWebSocket.instances.push(this);
  }

  addEventListener(type: string, callback: EventListenerOrEventListenerObject | null) {
    if (!callback) return;
    const handler: EventListener =
      typeof callback === "function" ? callback : (event) => callback.handleEvent(event);
    const handlers = this.listeners.get(type) ?? new Set<EventListener>();
    handlers.add(handler);
    this.listeners.set(type, handlers);
  }

  removeEventListener(type: string, callback: EventListenerOrEventListenerObject | null) {
    if (typeof callback === "function") this.listeners.get(type)?.delete(callback);
  }

  dispatchEvent(event: Event): boolean {
    this.listeners.get(event.type)?.forEach((listener) => listener(event));
    return true;
  }

  close() {
    this.readyState = TestWebSocket.CLOSED;
  }

  send() {
    throw new Error("The Agent Room event stream is server-push only.");
  }

  emitOpen() {
    this.readyState = TestWebSocket.OPEN;
    this.dispatchEvent(new Event("open"));
  }

  emitMessage(data: unknown) {
    this.dispatchEvent(new MessageEvent("message", { data: JSON.stringify(data) }));
  }

  emitClose(code = 1006) {
    this.readyState = TestWebSocket.CLOSED;
    this.dispatchEvent(new CloseEvent("close", { code }));
  }
}

class MemoryStorage implements Storage {
  private values = new Map<string, string>();
  get length() { return this.values.size; }
  clear() { this.values.clear(); }
  getItem(key: string) { return this.values.get(key) ?? null; }
  key(index: number) { return [...this.values.keys()][index] ?? null; }
  removeItem(key: string) { this.values.delete(key); }
  setItem(key: string, value: string) { this.values.set(key, String(value)); }
}

Object.defineProperty(window, "WebSocket", { value: TestWebSocket, writable: true });
Object.defineProperty(globalThis, "WebSocket", { value: TestWebSocket, writable: true });
Object.defineProperty(globalThis, "__testSockets", {
  get: () => TestWebSocket.instances,
  configurable: true,
});
const local = new MemoryStorage();
const session = new MemoryStorage();
Object.defineProperty(window, "localStorage", { value: local, configurable: true });
Object.defineProperty(window, "sessionStorage", { value: session, configurable: true });
Object.defineProperty(globalThis, "localStorage", {
  value: local,
  configurable: true,
});
Object.defineProperty(globalThis, "sessionStorage", {
  value: session,
  configurable: true,
});
Object.defineProperty(window.HTMLElement.prototype, "scrollIntoView", {
  value: vi.fn(),
  writable: true,
});

beforeEach(() => {
  TestWebSocket.instances.length = 0;
  localStorage.clear();
  sessionStorage.clear();
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});
