import { StrictMode } from "react";
import type { ReactNode } from "react";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { _roots, advance } from "@react-three/fiber";
import type { RootState } from "@react-three/fiber";
import type * as FiberModule from "@react-three/fiber";
import { Box3, Euler, Mesh, MeshBasicMaterial, Vector3 } from "three";
import type { Object3D } from "three";
import type * as ThreeModule from "three";
import type { CoordinationRequest, Project } from "../api/types";
import type { PortfolioSignal } from "./data";
import { agent, artifact, attention, evidence, run, task } from "../test/fixtures";
import { SceneCanvas } from "./SceneCanvas";
import type { SceneCanvasProps } from "./SceneCanvas";

const harness = vi.hoisted(() => ({
  rendererError: false, gpuError: false, gpuLoss: undefined as (() => void) | undefined,
  errors: [] as unknown[], canvases: [] as HTMLCanvasElement[],
  textDraws: [] as { canvas: HTMLCanvasElement; text: string; left: number; top: number; right: number; bottom: number; size: number }[],
  destroyed: vi.fn(), ended: vi.fn(),
}));

vi.mock("three", async (importOriginal) => {
  const three = await importOriginal<typeof ThreeModule>();
  class TestRenderer {
    domElement: HTMLCanvasElement;
    shadowMap = { enabled: false, type: 0 };
    renderLists = { dispose: vi.fn() };
    outputColorSpace = "";
    toneMapping = 0;
    xr = Object.assign(new EventTarget(), {
      enabled: false, isPresenting: false,
      getCamera: () => new three.PerspectiveCamera(),
      getReferenceSpace: () => ({}),
      setSession: vi.fn(), setAnimationLoop: vi.fn(), disconnect: vi.fn(),
      setReferenceSpaceType: vi.fn(), setFoveation: vi.fn(),
    });
    constructor(options: { canvas: HTMLCanvasElement }) {
      if (harness.rendererError) throw new Error("renderer unavailable");
      this.domElement = options.canvas;
    }
    render = vi.fn();
    setPixelRatio = vi.fn();
    getPixelRatio = () => 1;
    setSize = vi.fn();
    setClearColor = vi.fn();
    setClearAlpha = vi.fn();
    getClearAlpha = () => 1;
    dispose = vi.fn();
    forceContextLoss = vi.fn();
  }
  return { ...three, WebGLRenderer: TestRenderer };
});

vi.mock("three/webgpu", async () => {
  const { WebGLRenderer } = await import("three");
  class TestGPU extends WebGLRenderer {
    backend = { isWebGPUBackend: true, device: { lost: new Promise<void>((resolve) => { harness.gpuLoss = resolve; }) } };
    init() { return harness.gpuError ? Promise.reject(new Error("no adapter")) : Promise.resolve(); }
  }
  return { WebGPURenderer: TestGPU };
});

vi.mock("@react-three/fiber", async (importOriginal) => {
  const fiber = await importOriginal<typeof FiberModule>();
  const React = await import("react");
  const three = await import("three");
  fiber.extend({
    Group: three.Group, Mesh: three.Mesh, InstancedMesh: three.InstancedMesh,
    AmbientLight: three.AmbientLight, DirectionalLight: three.DirectionalLight, PointLight: three.PointLight,
    PlaneGeometry: three.PlaneGeometry, TorusGeometry: three.TorusGeometry, RingGeometry: three.RingGeometry,
    OctahedronGeometry: three.OctahedronGeometry, CylinderGeometry: three.CylinderGeometry, CapsuleGeometry: three.CapsuleGeometry,
    MeshBasicMaterial: three.MeshBasicMaterial, MeshStandardMaterial: three.MeshStandardMaterial,
  });
  function TestCanvas(props: {
    children: ReactNode; gl?: Parameters<ReturnType<typeof fiber.createRoot>["configure"]>[0] extends infer P ? P extends { gl?: infer G } ? G : never : never;
    onCreated?: (state: RootState) => void; camera?: { position?: [number, number, number]; fov?: number };
  }) {
    const canvas = React.useRef<HTMLCanvasElement>(null);
    const root = React.useRef<ReturnType<typeof fiber.createRoot> | null>(null);
    const initial = React.useRef(props);
    React.useEffect(() => {
      if (!canvas.current) return;
      harness.canvases.push(canvas.current);
      const current = fiber.createRoot(canvas.current);
      root.current = current;
      void current.configure({
        gl: typeof initial.current.gl === "function" ? initial.current.gl : new three.WebGLRenderer({ canvas: canvas.current }), size: { width: 1200, height: 600, top: 0, left: 0 },
        frameloop: "never", camera: initial.current.camera, onCreated: initial.current.onCreated,
      }).then(() => { current.render(initial.current.children); }).catch((error: unknown) => { harness.errors.push(error); });
      return () => { current.unmount(); };
    }, []);
    React.useEffect(() => { root.current?.render(props.children); }, [props.children]);
    return React.createElement("canvas", { ref: canvas, "data-testid": "scene-canvas" });
  }
  return { ...fiber, Canvas: TestCanvas };
});

interface TestXRStore {
  state: { session?: XRSession };
  listeners: Set<(state: { session?: XRSession }) => void>;
  subscribe: (listener: (state: { session?: XRSession }) => void) => () => void;
  destroy: () => void;
}

vi.mock("@react-three/xr", async () => {
  const React = await import("react");
  const { useThree } = await import("@react-three/fiber");
  const Context = React.createContext<TestXRStore | null>(null);
  return {
    createXRStore: (): TestXRStore => {
      const store: TestXRStore = {
        state: {}, listeners: new Set(), destroy: harness.destroyed,
        subscribe: (listener) => { store.listeners.add(listener); return () => { store.listeners.delete(listener); }; },
      };
      return store;
    },
    XR: ({ store, children }: { store: TestXRStore; children: ReactNode }) => {
      const manager = useThree((state) => state.gl.xr);
      manager.setSession = (session: XRSession | null) => {
        store.state = { session: session ?? undefined };
        manager.isPresenting = session !== null;
        store.listeners.forEach((listener) => { listener(store.state); });
        session?.addEventListener("end", () => {
          store.state = {};
          manager.isPresenting = false;
          store.listeners.forEach((listener) => { listener(store.state); });
        });
        return Promise.resolve();
      };
      return React.createElement(Context.Provider, { value: store }, children);
    },
    useXR: <T,>(selector: (state: { session?: XRSession }) => T): T => {
      const store = React.useContext(Context);
      if (!store) throw new Error("Missing XR test context");
      const state = React.useSyncExternalStore(store.subscribe, () => store.state);
      return selector(state);
    },
  };
});

const project: Project = { id: task.project_id, name: "Observatory", version: 1, updated_at: task.updated_at, capabilities: [] };
const props: SceneCanvasProps = {
  projects: [project], activeProjectId: project.id,
  tasks: [task, { ...task, id: "dependent", dependencies: [task.id], status: "blocked" }],
  runs: [run], attention: [attention], selectedTaskId: task.id,
  onProjectSelect: vi.fn(), onTaskSelect: vi.fn(), onBack: vi.fn(), reducedMotion: false,
  evidence: [evidence], artifacts: [{ ...artifact, task_id: task.id }],
};

function currentStates(): RootState[] {
  return harness.canvases.flatMap((canvas) => {
    const root = _roots.get(canvas);
    return root && document.contains(canvas) ? [root.store.getState()] : [];
  });
}

function objects(state: RootState): Object3D[] {
  const result: Object3D[] = [];
  state.scene.traverse((object) => { result.push(object); });
  return result;
}

function dispatch(object: Object3D | undefined, name: string, event: object = {}) {
  expect(object).toBeDefined();
  const instance = object as Object3D & { __r3f: { handlers: Record<string, (event: object) => void> } };
  const handler = instance.__r3f.handlers[name];
  expect(handler).toBeTypeOf("function");
  handler?.({ stopPropagation: vi.fn(), ...event });
}

function descendsFrom(object: Object3D, ancestor: Object3D): boolean {
  for (let parent = object.parent; parent; parent = parent.parent) if (parent === ancestor) return true;
  return false;
}

function labelsInside(state: RootState, ancestor: Object3D): Object3D[] {
  return objects(state).filter((object) => typeof object.userData.label === "string" && descendsFrom(object, ancestor));
}

function overlaps(a: Box3, b: Box3): boolean {
  return Math.min(a.max.x, b.max.x) - Math.max(a.min.x, b.min.x) > 0.001 && Math.min(a.max.y, b.max.y) - Math.max(a.min.y, b.min.y) > 0.001;
}

function assertInformationLayout(state: RootState) {
  state.scene.updateMatrixWorld(true);
  const all = objects(state);
  const group = (kind: string) => {
    const found = all.find((object) => object.userData.kind === kind);
    expect(found, kind).toBeDefined();
    return found!;
  };
  const labelBounds = all.filter((object) => typeof object.userData.label === "string").map((object) => ({ title: String(object.userData.label), bounds: new Box3().setFromObject(object) }));
  const handle = group("workspace-handle");
  labelBounds.push({ title: "Reposition handle", bounds: new Box3().setFromObject(handle) });
  for (let first = 0; first < labelBounds.length; first += 1) for (let second = first + 1; second < labelBounds.length; second += 1) {
    const a = labelBounds[first]!;
    const b = labelBounds[second]!;
    expect(overlaps(a.bounds, b.bounds), `${a.title} overlaps ${b.title}`).toBe(false);
  }
  const boundsOf = (kind: string) => labelsInside(state, group(kind)).reduce((box, label) => box.union(new Box3().setFromObject(label)), new Box3());
  const graph = boundsOf("workspace-sculpture");
  for (const object of all.filter((candidate) => ["workspace-nodes", "agent-participant", "portfolio-region", "active-run-signal", "selected-node-indicator"].includes(String(candidate.userData.kind)))) graph.union(new Box3().setFromObject(object));
  const left = boundsOf("coordination-surface");
  const right = boundsOf("human-escalations");
  const navigation = boundsOf("workspace-navigation");
  const diagnostics = boundsOf("workspace-diagnostics");
  expect(graph.min.x - left.max.x).toBeGreaterThan(0.05);
  expect(right.min.x - graph.max.x).toBeGreaterThan(0.05);
  expect(navigation.min.y - graph.max.y).toBeGreaterThan(0.05);
  expect(graph.min.y - diagnostics.max.y).toBeGreaterThan(0.05);
  const origin = group("workspace-placement").getWorldPosition(new Vector3());
  for (const { title, bounds } of labelBounds) {
    expect(Math.max(Math.abs(bounds.min.x - origin.x), Math.abs(bounds.max.x - origin.x)), title).toBeLessThan(1.6);
    expect(bounds.min.y - origin.y, title).toBeGreaterThan(-1.25);
    expect(bounds.max.y - origin.y, title).toBeLessThan(1.05);
  }
}

async function ready() {
  await waitFor(() => { expect(screen.getByText("WebGL 2")).toBeInTheDocument(); });
  await waitFor(() => { expect(currentStates()[0]?.scene.children.length).toBeGreaterThan(0); });
  expect(harness.errors).toEqual([]);
  return currentStates()[0]!;
}

beforeEach(() => {
  harness.rendererError = false;
  harness.gpuError = false;
  harness.gpuLoss = undefined;
  harness.errors.length = 0;
  harness.canvases.length = 0;
  harness.textDraws.length = 0;
  harness.destroyed.mockClear();
  vi.stubGlobal("WebGL2RenderingContext", class {});
  vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockImplementation(function (this: HTMLCanvasElement) {
    const context = {
      font: "16px system-ui", beginPath: vi.fn(), roundRect: vi.fn(), fill: vi.fn(), stroke: vi.fn(), fillRect: vi.fn(),
      measureText(value: string) {
        const size = Number(context.font.match(/(\d+)px/)?.[1] ?? 16);
        const width = [...value].reduce((sum, character) => sum + (character === "W" ? 0.97 : character === "i" ? 0.25 : 0.53) * size, 0);
        return { width, actualBoundingBoxLeft: 2, actualBoundingBoxRight: width + 2, actualBoundingBoxAscent: size * 0.8, actualBoundingBoxDescent: size * (/[gjpqy]/.test(value) ? 0.22 : 0.07) };
      },
      fillText: (value: string, x: number, y: number) => {
        const metrics = context.measureText(value);
        harness.textDraws.push({ canvas: this, text: value, left: x - metrics.actualBoundingBoxLeft, top: y - metrics.actualBoundingBoxAscent, right: x + metrics.actualBoundingBoxRight, bottom: y + metrics.actualBoundingBoxDescent, size: Number(context.font.match(/(\d+)px/)?.[1] ?? 16) });
      },
      createRadialGradient: () => ({ addColorStop: vi.fn() }),
    };
    return context as unknown as CanvasRenderingContext2D;
  });
});

afterEach(() => { vi.unstubAllGlobals(); vi.unstubAllEnvs(); });

describe("spatial workspace lifecycle with a CPU scene renderer", () => {
  it("keeps simultaneous dense task/request information, human escalations, diagnostics and controls disjoint and fully pageable", async () => {
    const session = new EventTarget() as EventTarget & XRSession;
    session.end = () => { session.dispatchEvent(new Event("end")); return Promise.resolve(); };
    Object.defineProperty(navigator, "xr", { value: { isSessionSupported: vi.fn().mockResolvedValue(true), requestSession: vi.fn().mockResolvedValue(session) }, configurable: true });
    vi.stubGlobal("isSecureContext", true);
    const longTitle = `Canonical title ${"W".repeat(64)} with the full independently recorded task identity.`;
    const description = `Complete human task description. ${"Review the provider evidence carefully. ".repeat(8)}End of canonical description.`;
    const denseTask = { ...task, title: longTitle, description, dependencies: ["cycle", "unavailable"] };
    const tasks = [denseTask, { ...task, id: "cycle", dependencies: [task.id] }, ...Array.from({ length: 28 }, (_, index) => ({ ...task, id: `dense-${index}`, title: `Recorded work ${index}`, dependencies: [] }))];
    const agents = Array.from({ length: 8 }, (_, index) => ({ ...agent, id: index ? `participant${index}` : agent.id, display_name: `Actual participant ${index}`, status: "idle" as const }));
    const human: PortfolioSignal = { project_id: project.id, project_name: project.name, kind: "request", resource_type: "coordination_request", id: "human_context", title: "Human context is explicitly requested", detail: `Recorded escalation reason. ${"Only the named recipient can provide this context. ".repeat(5)}`, status: "open", owner_id: agent.id, owner_name: "Builder", recipient: { type: "human", id: "human_operator" }, recipient_name: "Michael", updated_at: task.updated_at, human_escalation: true, source_system: "codex" };
    const work: PortfolioSignal = { ...human, id: "actual_work", kind: "run", resource_type: "run", title: "Actual provider verification", recipient: undefined, status: "running", human_escalation: false };
    const request: CoordinationRequest = { ...task, id: "long_request", kind: "handoff", title: `Request ${"W".repeat(64)} title remains available in full`, body: `Full handoff body. ${"Review the complete provider report. ".repeat(8)}End of request.`, owner_id: agent.id, owner_type: "agent", recipient: { type: "agent", id: "reviewer" }, evidence_ids: [evidence.id], status: "resolved", response: { actor_id: "reviewer", body: "Response evidence is verified.", evidence_ids: [evidence.id], responded_at: task.updated_at }, resolution: { actor_id: agent.id, note: "Handoff accepted after inspection.", closed_at: task.updated_at } };
    const inspect = vi.fn();
    const common = { ...props, tasks, agents, requests: [request], signals: [work, human], onSignalSelect: inspect };
    const view = render(<SceneCanvas {...common} />);
    await ready();
    fireEvent.click(await screen.findByRole("button", { name: "Enter immersive" }));
    await waitFor(() => expect(currentStates()[1]?.scene.children.length).toBeGreaterThan(0));
    const state = currentStates()[1]!;
    const cameraPosition = state.camera.position.clone();
    assertInformationLayout(state);
    const taskGroup = objects(state).find((object) => object.userData.kind === "task-inspector")!;
    const pageLabel = () => labelsInside(state, taskGroup).find((object) => /^review · \d+\//.test(String(object.userData.detail)))!;
    const count = Number(String(pageLabel().userData.detail).split("/").at(-1));
    const read: { title: string; body: string }[] = [];
    for (let index = 0; index < count; index += 1) {
      read.push({ title: String(pageLabel().userData.label), body: String(labelsInside(state, taskGroup).find((object) => object.userData.detail === "Recorded task and evidence")?.userData.label) });
      act(() => dispatch(labelsInside(state, taskGroup).find((object) => object.userData.label === "Next page"), "onClick"));
    }
    const compact = (value: string) => value.replace(/\s+/g, "");
    expect(compact(read.filter((page) => page.title === "Full task title").map((page) => page.body).join(""))).toBe(compact(longTitle));
    expect(compact(read.filter((page) => page.title === longTitle).map((page) => page.body).join(""))).toBe(compact(description));
    const humans = objects(state).find((object) => object.userData.kind === "human-escalations")!;
    const humanHeader = () => labelsInside(state, humans).find((object) => object.userData.label === "Human escalations")!;
    const humanCount = Number(String(humanHeader().userData.detail).match(/\d+\/(\d+)/)?.[1]);
    const humanPages: string[] = [];
    for (let index = 0; index < humanCount; index += 1) {
      const body = labelsInside(state, humans).find((object) => object.userData.detail === `${project.name} · open`)!;
      humanPages.push(String(body.userData.label));
      if (index === 1) act(() => dispatch(body, "onClick"));
      act(() => dispatch(humanHeader(), "onClick"));
    }
    expect(inspect).toHaveBeenCalledWith(human);
    expect(compact(humanPages.join(""))).toContain(compact(human.detail!));
    expect(compact(humanPages.join(""))).toContain(compact(human.updated_at));
    view.rerender(<SceneCanvas {...common} selectedTaskId={null} selectedRequestId={request.id} />);
    await waitFor(() => expect(objects(state).some((object) => object.userData.kind === "request-inspector")).toBe(true));
    assertInformationLayout(state);
    const requestGroup = objects(state).find((object) => object.userData.kind === "request-inspector")!;
    const requestHeader = () => labelsInside(state, requestGroup).find((object) => object.userData.label === "Coordination request")!;
    const requestCount = Number(String(requestHeader().userData.detail).split("/").at(-1));
    const requestPages: string[] = [];
    for (let index = 0; index < requestCount; index += 1) {
      requestPages.push(String(labelsInside(state, requestGroup).find((object) => object.userData.detail === "Respond in the project inspector")?.userData.label));
      act(() => dispatch(labelsInside(state, requestGroup).find((object) => object.userData.label === "Next request page"), "onClick"));
    }
    const fullRequest = compact(requestPages.join(""));
    expect(fullRequest).toContain(compact(request.title));
    expect(fullRequest).toContain(compact(request.body));
    expect(fullRequest).toContain(compact(request.resolution!.note));
    expect(fullRequest).toContain(`Responseevidencereference:${evidence.id}`);
    act(() => {
      state.camera.quaternion.setFromEuler(new Euler(0.12, 0.2, 0));
      advance(2, true, state);
      objects(state).filter((object) => object.userData.kind === "active-run-signal").forEach((object) => object.scale.setScalar(1.09));
    });
    assertInformationLayout(state);
    expect(state.camera.position).toEqual(cameraPosition);
    const canvases = new Set(harness.textDraws.map((draw) => draw.canvas));
    for (const canvas of canvases) {
      const ink = harness.textDraws.filter((draw) => draw.canvas === canvas);
      for (const draw of ink) {
        expect(draw.left, draw.text).toBeGreaterThanOrEqual(51.99);
        expect(draw.right, draw.text).toBeLessThanOrEqual(736.01);
        expect(draw.top, draw.text).toBeGreaterThanOrEqual(23.99);
        expect(draw.bottom, draw.text).toBeLessThanOrEqual(242.01);
      }
      for (let index = 1; index < ink.length; index += 1) expect(ink[index]!.top - ink[index - 1]!.bottom, ink[index]!.text).toBeGreaterThan(0);
    }
    const wideReading = harness.textDraws.filter((draw) => /^W{16}$/.test(draw.text));
    expect(wideReading.length).toBeGreaterThan(0);
    expect(wideReading.filter((draw) => draw.size < 40).map((draw) => ({ size: draw.size, lines: harness.textDraws.filter((line) => line.canvas === draw.canvas).map((line) => line.text) }))).toEqual([]);
    Reflect.deleteProperty(navigator, "xr");
  });

  it("renders four semantic region hubs and routes expand, collapse and page picks through shared controls", async () => {
    const choose = vi.fn();
    const paginate = vi.fn();
    const { rerender } = render(<SceneCanvas {...props} activeProjectId={null} portfolio={{}} region={null} onRegionSelect={choose} onRegionPageChange={paginate} />);
    const state = await ready();
    expect(objects(state).filter((object) => object.userData.kind === "portfolio-region")).toHaveLength(4);
    const floor = objects(state).find((object) => object instanceof Mesh && object.material instanceof MeshBasicMaterial && object.material.opacity === 0.045) as Mesh;
    expect((floor.material as MeshBasicMaterial).depthWrite).toBe(false);
    expect(objects(state).some((object) => object.userData.label === "? Coverage unknown")).toBe(true);
    act(() => { dispatch(objects(state).find((object) => object.userData.region === "coordination"), "onClick"); });
    expect(choose).toHaveBeenCalledWith("coordination");
    const many = Array.from({ length: 30 }, (_, index) => ({ ...project, id: `project${index}` }));
    rerender(<SceneCanvas {...props} projects={many} activeProjectId={null} portfolio={{}} region="unknown" page={1} onRegionSelect={choose} onRegionPageChange={paginate} />);
    await waitFor(() => expect(objects(state).some((object) => object.userData.label === "Previous projects")).toBe(true));
    act(() => { dispatch(objects(state).find((object) => object.userData.label === "Previous projects"), "onClick"); });
    expect(paginate).toHaveBeenCalledWith(0);
    act(() => { dispatch(objects(state).find((object) => object.userData.label === "Next projects"), "onClick"); });
    expect(paginate).toHaveBeenCalledWith(2);
    act(() => { dispatch(objects(state).find((object) => object.userData.label === "? Coverage unknown"), "onClick"); });
    expect(choose).toHaveBeenCalledWith(null);
  });

  it("renders named actual agents separately from context crystals and follows their recorded signal", async () => {
    const signal: PortfolioSignal = { project_id: project.id, project_name: project.name, kind: "agent", resource_type: "agent", id: agent.id, title: "Builder available", owner_name: agent.display_name, status: "idle", updated_at: agent.updated_at, human_escalation: false };
    const inspect = vi.fn();
    render(<SceneCanvas {...props} agents={[{ ...agent, status: "idle" }]} signals={[signal]} onSignalSelect={inspect} />);
    const state = await ready();
    expect(objects(state).some((object) => object.userData.label === "Agent · Builder")).toBe(true);
    act(() => { dispatch(objects(state).find((object) => object.userData.kind === "agent-participant"), "onClick"); });
    expect(inspect).toHaveBeenCalledWith(signal);
  });

  it("shows separate in-world Coordination and Human Escalations with actual attribution and full request pages", async () => {
    const session = new EventTarget() as EventTarget & XRSession;
    session.end = () => { session.dispatchEvent(new Event("end")); return Promise.resolve(); };
    Object.defineProperty(navigator, "xr", { value: { isSessionSupported: vi.fn().mockResolvedValue(true), requestSession: vi.fn().mockResolvedValue(session) }, configurable: true });
    vi.stubGlobal("isSecureContext", true);
    const request: CoordinationRequest = { ...task, id: "request1", kind: "handoff", title: "Review provider handoff", body: `Read the entire recorded handoff. ${"The actual evidence is attached for verification. ".repeat(5)}End of handoff.`, owner_id: agent.id, owner_type: "agent", recipient: { type: "human", id: "human_operator" }, status: "responded", evidence_ids: [evidence.id], escalation_reason: "Human approval is explicitly required.", response: { actor_id: "human_operator", body: "Reviewed against the provider record.", evidence_ids: [], responded_at: task.updated_at } };
    const blocker: PortfolioSignal = { project_id: project.id, project_name: project.name, kind: "blocker", resource_type: "task", id: task.id, title: "Recorded dependency wait", detail: "Awaiting the upstream provider trace.", status: "source:Blocked", owner_name: "Builder", owner_id: agent.id, updated_at: task.updated_at, human_escalation: false, source_system: "sentry" };
    const human: PortfolioSignal = { ...blocker, kind: "request", resource_type: "coordination_request", id: request.id, title: request.title, detail: request.escalation_reason, recipient: request.recipient, recipient_name: "Michael", human_escalation: true };
    const inspect = vi.fn();
    const view = render(<SceneCanvas {...props} selectedTaskId={null} requests={[request]} signals={[blocker, human]} onSignalSelect={inspect} />);
    await ready();
    fireEvent.click(await screen.findByRole("button", { name: "Enter immersive" }));
    await waitFor(() => expect(currentStates()[1]?.scene.children.length).toBeGreaterThan(0));
    const state = currentStates()[1]!;
    expect(objects(state).some((object) => object.userData.kind === "coordination-surface")).toBe(true);
    expect(objects(state).some((object) => object.userData.kind === "human-escalations")).toBe(true);
    const body = objects(state).find((object) => String(object.userData.label).startsWith("Builder → Michael"));
    act(() => { dispatch(body, "onClick"); });
    expect(inspect).toHaveBeenCalledWith(human);
    const turn = objects(state).find((object) => object.userData.label === "Next coordination page");
    act(() => { dispatch(turn, "onClick"); });
    view.rerender(<SceneCanvas {...props} selectedTaskId={null} selectedRequestId={request.id} requests={[request]} signals={[blocker, human]} onSignalSelect={inspect} />);
    await waitFor(() => expect(objects(state).some((object) => object.userData.kind === "request-inspector")).toBe(true));
    const countLabel = () => objects(state).find((object) => object.userData.label === "Coordination request");
    const count = Number(String(countLabel()?.userData.detail).split("/").at(-1));
    const pages: string[] = [];
    for (let index = 0; index < count; index += 1) {
      pages.push(String(objects(state).find((object) => object.userData.detail === "Respond in the project inspector")?.userData.label));
      act(() => { dispatch(objects(state).find((object) => object.userData.label === "Next request page"), "onClick"); });
    }
    const all = pages.join("").replace(/\s+/g, "");
    expect(all).toContain(request.body.replace(/\s+/g, ""));
    expect(all).toContain("Humanapprovalisexplicitlyrequired.");
    expect(all).toContain("Reviewedagainsttheproviderrecord.");
    expect(all).toContain(`Evidencereference:${evidence.id}`);
    Reflect.deleteProperty(navigator, "xr");
  });

  it("retains an accessible fallback without a graphics API", async () => {
    vi.stubGlobal("WebGL2RenderingContext", undefined);
    render(<SceneCanvas {...props} />);
    expect(await screen.findByText(/Your workspace is ready/)).toBeInTheDocument();
    expect(screen.queryByTestId("scene-canvas")).not.toBeInTheDocument();
  });

  it("builds actual Three scene instances, picks canonical tasks, and navigates on demand", async () => {
    render(<SceneCanvas {...props} />);
    const state = await ready();
    const nodes = objects(state).find((object) => object.userData.kind === "workspace-nodes");
    act(() => { dispatch(nodes, "onClick", { instanceId: 1 }); });
    expect(props.onTaskSelect).toHaveBeenCalledWith(task.id);
    act(() => { dispatch(nodes, "onClick", { instanceId: 0 }); });
    expect(props.onProjectSelect).toHaveBeenCalledWith(project.id);
    act(() => { dispatch(nodes, "onPointerOver", { instanceId: 1 }); dispatch(nodes, "onPointerOut"); });
    const canvas = screen.getByTestId("scene-canvas");
    const position = state.camera.position.clone();
    act(() => {
      fireEvent.wheel(canvas, { deltaY: -400 });
      fireEvent.pointerDown(canvas, { button: 0, clientX: 10, clientY: 10, pointerId: 1 });
      fireEvent.pointerMove(window, { clientX: 50, clientY: 20, pointerId: 1 });
      fireEvent.pointerUp(window);
      advance(1, true, state);
    });
    expect(state.camera.position.equals(position)).toBe(false);
    fireEvent.click(screen.getByRole("button", { name: "Recenter view" }));
    expect(objects(state).some((object) => object.userData.kind === "workspace-dependencies")).toBe(true);
    expect(objects(state).some((object) => object.userData.kind === "active-run-signal")).toBe(true);
  });

  it("recovers from WebGPU initialization failure and context loss with preserved DOM access", async () => {
    Object.defineProperty(navigator, "gpu", { value: {}, configurable: true });
    harness.gpuError = true;
    const view = render(<SceneCanvas {...props} reducedMotion />);
    await ready();
    expect(screen.getByRole("status")).toHaveTextContent("WebGPU is unavailable");
    fireEvent(screen.getByTestId("scene-canvas"), new Event("webglcontextlost", { cancelable: true }));
    expect(await screen.findByText(/Your workspace is ready/)).toBeInTheDocument();
    view.unmount();
    Reflect.deleteProperty(navigator, "gpu");
  });

  it("labels the actual GPU backend and switches to WebGL after device loss", async () => {
    Object.defineProperty(navigator, "gpu", { value: {}, configurable: true });
    render(<SceneCanvas {...props} />);
    expect(await screen.findByText("WebGPU")).toBeInTheDocument();
    act(() => { harness.gpuLoss?.(); });
    await ready();
    expect(screen.getByRole("status")).toHaveTextContent("GPU device was lost");
    Reflect.deleteProperty(navigator, "gpu");
  });

  it("recovers cancelled headset entry and shares selection/evidence with a real spatial scene", async () => {
    const session = new EventTarget() as EventTarget & XRSession;
    const end = vi.fn(() => { session.dispatchEvent(new Event("end")); return Promise.resolve(); });
    session.end = end;
    const request = vi.fn().mockRejectedValueOnce(new DOMException("cancelled", "NotAllowedError")).mockResolvedValue(session);
    Object.defineProperty(navigator, "xr", { value: { isSessionSupported: vi.fn().mockResolvedValue(true), requestSession: request }, configurable: true });
    vi.stubGlobal("isSecureContext", true);
    render(<SceneCanvas {...props} />);
    await ready();
    const enter = await screen.findByRole("button", { name: "Enter immersive" });
    fireEvent.click(enter);
    expect(request).toHaveBeenCalledOnce();
    expect(await screen.findByRole("status")).toHaveTextContent("cancelled");
    fireEvent.click(enter);
    expect(await screen.findByText("WebXR · WebGL 2")).toBeInTheDocument();
    await waitFor(() => { expect(currentStates()[1]?.scene.children.length).toBeGreaterThan(0); });
    const xrState = currentStates()[1]!;
    expect(objects(xrState).some((object) => object.userData.label === task.title)).toBe(true);
    const next = objects(xrState).find((object) => object.userData.label === "Next page");
    act(() => { dispatch(next, "onClick"); });
    expect(objects(xrState).some((object) => object.userData.label === "Worker · working")).toBe(true);
    act(() => { dispatch(next, "onClick"); });
    expect(objects(xrState).some((object) => object.userData.label === `${evidence.kind} · ${evidence.source.system}`)).toBe(true);
    act(() => { dispatch(objects(xrState).find((object) => object.userData.label === "All projects"), "onClick"); });
    expect(props.onBack).toHaveBeenCalled();
    const source = { gripSpace: {} } as XRInputSource;
    const start = new Event("selectstart") as XRInputSourceEvent;
    Object.defineProperty(start, "inputSource", { value: source });
    act(() => {
      session.dispatchEvent(start);
      dispatch(objects(xrState).find((object) => object.userData.kind === "workspace-handle"), "onPointerDown");
      const frame = { getPose: () => ({ transform: { position: new Vector3(0.2, 1, -0.5) } }) } as unknown as XRFrame;
      advance(1, true, xrState, frame);
      advance(2, true, xrState, { getPose: () => ({ transform: { position: new Vector3(0.4, 1, -0.5) } }) } as unknown as XRFrame);
      dispatch(objects(xrState).find((object) => object.userData.kind === "workspace-handle"), "onPointerUp");
    });
    act(() => { dispatch(objects(xrState).find((object) => object.userData.label === "Exit immersive"), "onClick"); });
    expect(end).toHaveBeenCalledOnce();
    await waitFor(() => { expect(screen.queryByText("WebXR · WebGL 2")).not.toBeInTheDocument(); });
    Reflect.deleteProperty(navigator, "xr");
  });

  it("reads run-linked evidence and artifacts once, with complete references and project boundaries", async () => {
    const session = new EventTarget() as EventTarget & XRSession;
    session.end = () => { session.dispatchEvent(new Event("end")); return Promise.resolve(); };
    Object.defineProperty(navigator, "xr", { value: {
      isSessionSupported: vi.fn().mockResolvedValue(true), requestSession: vi.fn().mockResolvedValue(session),
    }, configurable: true });
    vi.stubGlobal("isSecureContext", true);
    const sourceURL = "https://example.org/tasks/source";
    const evidenceURL = "https://example.org/checks/result";
    const artifactURL = "https://example.org/artifacts/run.txt";
    render(<SceneCanvas {...props}
      tasks={[{ ...task, source: { ...task.source, external_url: sourceURL } }]}
      runs={[run, { ...run, id: "unrelated-run", task_id: "unrelated-task", summary: "Unrelated worker" },
        { ...run, id: "foreign-run", project_id: "another-project", summary: "Foreign worker" }]}
      evidence={[
        { ...evidence, id: "run-only", task_id: undefined, run_id: run.id, summary: "Run-only evidence", source: { ...evidence.source, external_url: evidenceURL } },
        { ...evidence, id: "both-links", run_id: run.id, summary: "Dual-linked evidence" },
        { ...evidence, id: "unrelated-evidence", task_id: undefined, run_id: "unrelated-run", summary: "Unrelated evidence" },
        { ...evidence, id: "foreign-evidence", project_id: "another-project", run_id: run.id, summary: "Foreign evidence" },
      ]}
      artifacts={[
        { ...artifact, id: "run-artifact", name: "run.txt", scope_type: "run", task_id: undefined, run_id: run.id, uri: artifactURL, media_type: "text/plain" },
        { ...artifact, id: "both-artifact", name: "dual.txt", task_id: task.id, run_id: run.id, uri: artifactURL, media_type: "text/plain" },
        { ...artifact, id: "unrelated-artifact", name: "unrelated.txt", task_id: undefined, run_id: "unrelated-run" },
        { ...artifact, id: "foreign-artifact", name: "foreign.txt", project_id: "another-project", task_id: task.id, run_id: run.id },
      ]} />);
    await ready();
    fireEvent.click(await screen.findByRole("button", { name: "Enter immersive" }));
    await waitFor(() => { expect(currentStates()[1]?.scene.children.length).toBeGreaterThan(0); });
    const state = currentStates()[1]!;
    const next = objects(state).find((object) => object.userData.label === "Next page");
    const pageLabel = () => objects(state).find((object) => typeof object.userData.detail === "string" && object.userData.detail.startsWith(`${task.status} · `) && object.userData.detail.includes("/"));
    const count = Number(String(pageLabel()?.userData.detail).split("/").at(-1));
    const seen: { title: string; detail: string }[] = [];
    for (let index = 0; index < count; index += 1) {
      const detail = objects(state).find((object) => object.userData.detail === "Recorded task and evidence");
      seen.push({ title: String(pageLabel()?.userData.label), detail: String(detail?.userData.label) });
      act(() => { dispatch(next, "onClick"); });
    }
    expect(seen.some((entry) => entry.detail === "Run-only evidence")).toBe(true);
    expect(seen.filter((entry) => entry.detail === "Dual-linked evidence")).toHaveLength(1);
    expect(seen.filter((entry) => entry.detail.startsWith("run.txt"))).toHaveLength(1);
    expect(seen.filter((entry) => entry.detail.startsWith("dual.txt"))).toHaveLength(1);
    const reference = (title: string) => seen.filter((entry) => entry.title === title).map((entry) => entry.detail).join("").replaceAll(" ", "");
    expect(reference("Task source reference")).toBe(sourceURL);
    expect(reference("Evidence source reference")).toBe(evidenceURL);
    expect(reference("Artifact reference")).toBe(artifactURL.repeat(2));
    expect(JSON.stringify(seen)).not.toMatch(/Unrelated|Foreign|unrelated\.txt|foreign\.txt/);
    Reflect.deleteProperty(navigator, "xr");
  });

  it("paginates the entire latest source report while retaining the human task fields", async () => {
    const session = new EventTarget() as EventTarget & XRSession;
    session.end = () => { session.dispatchEvent(new Event("end")); return Promise.resolve(); };
    Object.defineProperty(navigator, "xr", { value: { isSessionSupported: vi.fn().mockResolvedValue(true), requestSession: vi.fn().mockResolvedValue(session) }, configurable: true });
    vi.stubGlobal("isSecureContext", true);
    const sourceDescription = `Latest verification: 10/10 checks passed. ${"All source checks were independently recorded and reviewed. ".repeat(4)}Final source result: 10/10.`;
    const observedAt = "2026-10-14T12:00:00Z";
    const sourceReference = "file:///workspace/release.md";
    const supervised = { ...task, title: "Human release title", description: "Human review: 8/10 checks recorded.", source_status: "Done", source_observed_at: observedAt,
      source_observation: { external_id: "release-source", title: "Source release: 10/10", status: "Done", description: sourceDescription, external_url: sourceReference } };
    const before = JSON.stringify(supervised);
    render(<SceneCanvas {...props} tasks={[supervised]} runs={[]} evidence={[]} artifacts={[]} />);
    await ready();
    fireEvent.click(await screen.findByRole("button", { name: "Enter immersive" }));
    await waitFor(() => { expect(currentStates()[1]?.scene.children.length).toBeGreaterThan(0); });
    const state = currentStates()[1]!;
    const next = objects(state).find((object) => object.userData.label === "Next page");
    const pageLabel = () => objects(state).find((object) => typeof object.userData.detail === "string" && object.userData.detail.startsWith(`${task.status} · `) && object.userData.detail.includes("/"));
    const count = Number(String(pageLabel()?.userData.detail).split("/").at(-1));
    const seen: { title: string; detail: string }[] = [];
    for (let index = 0; index < count; index += 1) {
      const detail = objects(state).find((object) => object.userData.detail === "Recorded task and evidence");
      seen.push({ title: String(pageLabel()?.userData.label), detail: String(detail?.userData.label) });
      act(() => { dispatch(next, "onClick"); });
    }
    const text = (title: string) => seen.filter((entry) => entry.title === title).map((entry) => entry.detail).join("").replace(/\s+/g, "");
    expect(seen[0]).toEqual({ title: supervised.title, detail: supervised.description });
    expect(text("Latest source title")).toBe("Sourcerelease:10/10");
    expect(text("Latest source report")).toBe(sourceDescription.replace(/\s+/g, ""));
    expect(seen.filter((entry) => entry.title === "Latest source report").length).toBeGreaterThan(1);
    expect(text("Source report status")).toBe("Done·codex");
    expect(text("Source observed")).toBe(observedAt);
    expect(text("Source report reference")).toBe(sourceReference);
    expect(JSON.stringify(supervised)).toBe(before);
    Reflect.deleteProperty(navigator, "xr");
  });

  it("discloses missing dependency details and cycle-affected tasks in desktop and XR alongside errors", async () => {
    const session = new EventTarget() as EventTarget & XRSession;
    session.end = () => { session.dispatchEvent(new Event("end")); return Promise.resolve(); };
    const request = vi.fn().mockRejectedValueOnce(new DOMException("cancelled", "NotAllowedError")).mockResolvedValue(session);
    Object.defineProperty(navigator, "xr", { value: { isSessionSupported: vi.fn().mockResolvedValue(true), requestSession: request }, configurable: true });
    vi.stubGlobal("isSecureContext", true);
    render(<SceneCanvas {...props} tasks={[
      { ...task, dependencies: ["cycle-2", "missing-dependency"] },
      { ...task, id: "cycle-2", dependencies: [task.id] },
      { ...task, id: "downstream", dependencies: ["cycle-2"] },
    ]} />);
    await ready();
    const status = screen.getByRole("status");
    expect(status).toHaveTextContent("Dependency details unavailable: 1.");
    expect(status).toHaveTextContent("Tasks in or downstream of a dependency cycle: 3.");
    const enter = await screen.findByRole("button", { name: "Enter immersive" });
    fireEvent.click(enter);
    await waitFor(() => { expect(status).toHaveTextContent("cancelled"); });
    expect(status).toHaveTextContent("Dependency details unavailable: 1.");
    fireEvent.click(enter);
    await waitFor(() => { expect(currentStates()[1]?.scene.children.length).toBeGreaterThan(0); });
    const labels = objects(currentStates()[1]!).map((object) => ({ title: String(object.userData.label), detail: String(object.userData.detail) }));
    expect(labels).toContainEqual({ title: "Dependency details unavailable", detail: "1 declared links" });
    expect(labels).toContainEqual({ title: "Tasks in / downstream of cycle", detail: "3 tasks" });
    Reflect.deleteProperty(navigator, "xr");
  });

  it("recreates the XR store during StrictMode effect replay, then releases the live instance on unmount", async () => {
    vi.stubGlobal("WebGL2RenderingContext", undefined);
    const view = render(<StrictMode><SceneCanvas {...props} /></StrictMode>);
    await screen.findByText(/Your workspace is ready/);
    expect(harness.destroyed).toHaveBeenCalledOnce();
    view.unmount();
    await act(async () => { await Promise.resolve(); });
    expect(harness.destroyed).toHaveBeenCalledTimes(2);
  });
});
