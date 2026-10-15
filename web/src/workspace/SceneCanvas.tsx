import { Component, useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { ReactNode } from "react";
import { Canvas, useFrame, useThree } from "@react-three/fiber";
import type { RootState, ThreeEvent } from "@react-three/fiber";
import { createXRStore, XR, useXR } from "@react-three/xr";
import {
  AdditiveBlending, CanvasTexture, Color, Matrix4, Object3D,
  PerspectiveCamera, Quaternion, SRGBColorSpace, Vector3, WebGLRenderer,
} from "three";
import type { Group, InstancedMesh, Mesh, Texture, WebXRManager } from "three";
import type { Agent, Artifact, AttentionItem, CoordinationRequest, Evidence, Project, Run, Task } from "../api/types";
import type { PortfolioEntry, PortfolioRegion, PortfolioSignal } from "./data";
import { buildWorkspaceScene, shortLabel, textPages, visibleLabels, wrapLabel } from "./scene";
import type { Point3, SceneNode, WorkspaceScene } from "./scene";
import { gripGain, gripTranslation, requestImmersiveSession, sessionError } from "./xrInput";

export interface SceneCanvasProps {
  projects: Project[];
  activeProjectId: string | null;
  tasks: Task[];
  runs: Run[];
  attention: AttentionItem[];
  selectedTaskId: string | null;
  selectedRequestId?: string | null;
  onProjectSelect: (id: string) => void;
  onTaskSelect: (id: string) => void;
  onBack?: () => void;
  evidence?: Evidence[];
  artifacts?: Artifact[];
  agents?: Agent[];
  requests?: CoordinationRequest[];
  portfolio?: Record<string, PortfolioEntry>;
  region?: PortfolioRegion | null;
  page?: number;
  now?: number;
  signals?: PortfolioSignal[];
  onRegionSelect?: (region: PortfolioRegion | null) => void;
  onRegionPageChange?: (page: number) => void;
  onSignalSelect?: (signal: PortfolioSignal) => void;
  reducedMotion: boolean;
}

interface SceneContentProps extends SceneCanvasProps {
  projection: WorkspaceScene;
  immersive: boolean;
  animate: boolean;
  resetToken: number;
  onExit?: () => void;
}

const invisibleStage = {
  position: "fixed", width: 1, height: 1, top: 0, left: 0, visibility: "hidden", pointerEvents: "none",
} as const;

class GraphicsBoundary extends Component<{ children: ReactNode; onFailure: () => void }, { failed: boolean }> {
  state = { failed: false };
  static getDerivedStateFromError() { return { failed: true }; }
  componentDidCatch() { this.props.onFailure(); }
  render() { return this.state.failed ? null : this.props.children; }
}

function labelTexture(title: string, detail: string, color: string, prominent: boolean): CanvasTexture {
  const width = 768;
  const canvas = document.createElement("canvas");
  canvas.width = width;
  canvas.height = 256;
  const context = canvas.getContext("2d");
  if (!context) throw new Error("Canvas text rendering is unavailable");
  context.fillStyle = "rgba(7,14,29,0.9)";
  context.beginPath();
  context.roundRect(2, 2, width - 4, 252, 24);
  context.fill();
  context.strokeStyle = `${color}70`;
  context.lineWidth = 2;
  context.stroke();
  context.fillStyle = color;
  context.fillRect(28, 34, 5, 175);
  context.font = `500 ${prominent ? 72 : 58}px system-ui, sans-serif`;
  context.fillStyle = "#edf2ff";
  const lines = wrapLabel(title, prominent ? 18 : 22, 3);
  lines.forEach((line, index) => context.fillText(line, 52, 66 + index * 54));
  context.fillStyle = color;
  context.font = `400 ${prominent ? 40 : 32}px system-ui, sans-serif`;
  context.fillText(shortLabel(detail, prominent ? 32 : 36), 52, 224);
  const texture = new CanvasTexture(canvas);
  texture.colorSpace = SRGBColorSpace;
  return texture;
}

function useTextTexture(title: string, detail: string, color: string, prominent: boolean): Texture {
  const texture = useMemo(() => labelTexture(title, detail, color, prominent), [title, detail, color, prominent]);
  useEffect(() => () => texture.dispose(), [texture]);
  return texture;
}

function useLightTexture(): Texture {
  const texture = useMemo(() => {
    const canvas = document.createElement("canvas");
    canvas.width = canvas.height = 128;
    const context = canvas.getContext("2d");
    if (!context) throw new Error("Canvas light rendering is unavailable");
    const gradient = context.createRadialGradient(64, 64, 0, 64, 64, 64);
    gradient.addColorStop(0, "rgba(255,255,255,0.5)");
    gradient.addColorStop(0.18, "rgba(255,255,255,0.15)");
    gradient.addColorStop(0.5, "rgba(255,255,255,0.035)");
    gradient.addColorStop(1, "rgba(255,255,255,0)");
    context.fillStyle = gradient;
    context.fillRect(0, 0, 128, 128);
    const result = new CanvasTexture(canvas);
    result.colorSpace = SRGBColorSpace;
    return result;
  }, []);
  useEffect(() => () => texture.dispose(), [texture]);
  return texture;
}

function Label({ title, detail, color, position, width = 1.7, prominent = false, onClick }: {
  title: string; detail: string; color: string; position: Point3; width?: number; prominent?: boolean; onClick?: () => void;
}) {
  const texture = useTextTexture(title, detail, color, prominent);
  const mesh = useRef<Mesh>(null);
  useFrame(({ camera }) => { if (mesh.current) mesh.current.quaternion.copy(camera.quaternion); });
  return <mesh ref={mesh} position={position} userData={{ label: title, detail }} onClick={onClick ? (event) => { event.stopPropagation(); onClick(); } : undefined}>
    <planeGeometry args={[width, width / 3]} />
    <meshBasicMaterial map={texture} transparent toneMapped={false} depthWrite={false} />
  </mesh>;
}

/** Restrained environmental sculpture; never represents observations or activity. */
function Observatory() {
  return <group>
    <ambientLight intensity={1.1} color="#b6ccff" />
    <directionalLight position={[4, 6, 8]} intensity={2.2} color="#e3eeff" />
    <pointLight position={[-5, -1, 3]} intensity={24} distance={20} color="#537bff" />
    <pointLight position={[5, 3, 1]} intensity={18} distance={17} color="#77dbcb" />
    <mesh position={[0, 0, -4]} rotation={[0.18, 0.35, 0.28]}>
      <torusGeometry args={[9.4, 0.018, 6, 150, Math.PI * 1.65]} />
      <meshBasicMaterial color="#8eabef" transparent opacity={0.18} depthWrite={false} />
    </mesh>
    <mesh position={[0, 0, -4.5]} rotation={[-0.23, -0.35, -0.16]}>
      <torusGeometry args={[11.2, 0.012, 6, 150, Math.PI * 1.45]} />
      <meshBasicMaterial color="#8cf0d1" transparent opacity={0.14} depthWrite={false} />
    </mesh>
    <mesh position={[0, -5.4, -2]} rotation={[-Math.PI / 2, 0, 0]}>
      <ringGeometry args={[4, 12, 100]} />
      <meshBasicMaterial color="#617ab7" transparent opacity={0.045} depthWrite={false} />
    </mesh>
  </group>;
}

function Constellation({ nodes, selected, onSelect, onHover }: {
  nodes: SceneNode[]; selected: string | null; onSelect: (node: SceneNode) => void; onHover: (id: string | null) => void;
}) {
  const crystals = useRef<InstancedMesh>(null);
  const cores = useRef<InstancedMesh>(null);
  const halos = useRef<InstancedMesh>(null);
  const lightTexture = useLightTexture();
  const invalidate = useThree((state) => state.invalidate);
  useEffect(() => {
    const crystal = crystals.current;
    const core = cores.current;
    const halo = halos.current;
    if (!crystal || !core || !halo) return;
    const object = new Object3D();
    const color = new Color();
    nodes.forEach((node, index) => {
      object.position.fromArray(node.position);
      object.rotation.set(0.32, 0.7, Math.PI / 4);
      const size = node.kind === "project" ? 0.3 : 0.19;
      object.scale.setScalar(size * (selected === node.id ? 1.2 : 1));
      object.updateMatrix();
      crystal.setMatrixAt(index, object.matrix);
      crystal.setColorAt(index, color.set(node.color));
      object.scale.setScalar(size * 0.55);
      object.updateMatrix();
      core.setMatrixAt(index, object.matrix);
      core.setColorAt(index, color.set(node.color));
      object.rotation.set(0, 0, 0);
      object.position.z -= 0.05;
      object.scale.setScalar(node.kind === "project" ? 2.2 : 1.25);
      object.updateMatrix();
      halo.setMatrixAt(index, object.matrix);
      halo.setColorAt(index, color.set(node.color));
    });
    crystal.instanceMatrix.needsUpdate = true;
    core.instanceMatrix.needsUpdate = true;
    halo.instanceMatrix.needsUpdate = true;
    if (crystal.instanceColor) crystal.instanceColor.needsUpdate = true;
    if (core.instanceColor) core.instanceColor.needsUpdate = true;
    if (halo.instanceColor) halo.instanceColor.needsUpdate = true;
    crystal.computeBoundingSphere();
    core.computeBoundingSphere();
    halo.computeBoundingSphere();
    invalidate();
  }, [nodes, selected, invalidate]);
  const pick = (event: ThreeEvent<MouseEvent>) => {
    event.stopPropagation();
    const node = event.instanceId === undefined ? undefined : nodes[event.instanceId];
    if (node) onSelect(node);
  };
  return <group>
    <instancedMesh ref={halos} args={[undefined, undefined, nodes.length]} raycast={() => undefined}>
      <planeGeometry args={[1, 1]} />
      <meshBasicMaterial map={lightTexture} transparent depthWrite={false} toneMapped={false} blending={AdditiveBlending} />
    </instancedMesh>
    <instancedMesh ref={crystals} userData={{ kind: "workspace-nodes" }} args={[undefined, undefined, nodes.length]} onClick={pick}
      onPointerOver={(event) => { event.stopPropagation(); onHover(nodes[event.instanceId ?? -1]?.id ?? null); }}
      onPointerOut={() => onHover(null)}>
      <octahedronGeometry args={[1, 0]} />
      <meshStandardMaterial metalness={0.55} roughness={0.24} transparent opacity={0.78} />
    </instancedMesh>
    <instancedMesh ref={cores} args={[undefined, undefined, nodes.length]} raycast={() => undefined}>
      <octahedronGeometry args={[1, 0]} />
      <meshBasicMaterial toneMapped={false} />
    </instancedMesh>
  </group>;
}

function DependencyThreads({ projection, selected }: { projection: WorkspaceScene; selected: string | null }) {
  const geometry = useMemo(() => {
    const nodes = new Map(projection.nodes.map((node) => [node.id, node]));
    const result: { position: Point3; quaternion: Quaternion; length: number; color: string; emphasized: boolean }[] = [];
    const up = new Vector3(0, 1, 0);
    for (const edge of projection.edges) {
      const from = nodes.get(edge.from);
      const to = nodes.get(edge.to);
      if (!from || !to) continue;
      const start = new Vector3(...from.position);
      const end = new Vector3(...to.position);
      const direction = end.clone().sub(start);
      const length = direction.length();
      if (length === 0) continue;
      const midpoint = start.clone().add(end).multiplyScalar(0.5);
      result.push({ position: midpoint.toArray(), quaternion: new Quaternion().setFromUnitVectors(up, direction.normalize()),
        length, color: edge.relationship === "ownership" ? "#94e0e8" : edge.blocked ? "#ffb46e" : "#9ab5e8", emphasized: edge.from === selected || edge.to === selected });
    }
    return result;
  }, [projection, selected]);
  const mesh = useRef<InstancedMesh>(null);
  useEffect(() => {
    if (!mesh.current) return;
    const matrix = new Matrix4();
    const color = new Color();
    geometry.forEach((edge, index) => {
      matrix.compose(new Vector3(...edge.position), edge.quaternion, new Vector3(edge.emphasized ? 0.012 : 0.005, edge.length, edge.emphasized ? 0.012 : 0.005));
      mesh.current?.setMatrixAt(index, matrix);
      mesh.current?.setColorAt(index, color.set(edge.color).multiplyScalar(edge.emphasized ? 1 : 0.45));
    });
    mesh.current.instanceMatrix.needsUpdate = true;
    if (mesh.current.instanceColor) mesh.current.instanceColor.needsUpdate = true;
    mesh.current.computeBoundingSphere();
  }, [geometry]);
  return <instancedMesh ref={mesh} userData={{ kind: "workspace-dependencies" }} args={[undefined, undefined, geometry.length]} raycast={() => undefined}>
    <cylinderGeometry args={[1, 1, 1, 5]} />
    <meshBasicMaterial transparent opacity={0.75} toneMapped={false} />
  </instancedMesh>;
}

function SignalRing({ node, animate }: { node: SceneNode; animate: boolean }) {
  const ring = useRef<Mesh>(null);
  useFrame(({ clock, invalidate }) => {
    if (!animate || !ring.current) return;
    ring.current.scale.setScalar(1 + Math.sin(clock.elapsedTime * 1.6) * 0.09);
    invalidate();
  });
  return <mesh ref={ring} userData={{ kind: "active-run-signal", resource_id: node.id }} position={node.position} rotation={[0.22, -0.25, 0]} raycast={() => undefined}>
    <torusGeometry args={[node.kind === "project" ? 0.46 : 0.32, 0.012, 6, 32]} />
    <meshBasicMaterial color={node.color} transparent opacity={0.65} toneMapped={false} blending={AdditiveBlending} />
  </mesh>;
}

function updateCameraDepth(camera: PerspectiveCamera, far: number) {
  camera.far = far;
  camera.updateProjectionMatrix();
}

function DesktopNavigation({ nodes, resetToken, reducedMotion }: { nodes: SceneNode[]; resetToken: number; reducedMotion: boolean }) {
  const { camera, gl, invalidate, size } = useThree();
  const target = useRef(new Vector3(0, 0, 15));
  const maxZoom = useRef(90);
  const hasFit = useRef(false);
  const drag = useRef<{ x: number; y: number; pointer: number } | null>(null);
  useEffect(() => {
    if (!(camera instanceof PerspectiveCamera)) return;
    const minX = Math.min(-1, ...nodes.map((node) => node.position[0] - (node.kind === "region" ? 2.1 : 1.35)));
    const maxX = Math.max(1, ...nodes.map((node) => node.position[0] + (node.kind === "region" ? 2.1 : 1.35)));
    const minY = Math.min(-1, ...nodes.map((node) => node.position[1] - (node.action === "expand" ? 1.8 : 1.2)));
    const maxY = Math.max(1, ...nodes.map((node) => node.position[1] + (node.action === "collapse" ? 1.4 : 0.65)));
    const vertical = (maxY - minY) / 2;
    const horizontal = (maxX - minX) / 2 / Math.max(size.width / size.height, 0.2);
    const distance = Math.max(3.8, Math.max(vertical, horizontal) / Math.tan(camera.fov * Math.PI / 360) * 1.13);
    maxZoom.current = Math.max(90, distance * 2);
    updateCameraDepth(camera, Math.max(180, maxZoom.current * 2));
    target.current.set((minX + maxX) / 2, (minY + maxY) / 2, distance);
    if (reducedMotion || !hasFit.current) camera.position.copy(target.current);
    hasFit.current = true;
    invalidate();
  }, [camera, nodes, resetToken, size.width, size.height, reducedMotion, invalidate]);
  useEffect(() => {
    const canvas = gl.domElement;
    const wheel = (event: WheelEvent) => {
      event.preventDefault();
      target.current.z = Math.max(3.5, Math.min(maxZoom.current, target.current.z * Math.exp(event.deltaY * 0.001)));
      invalidate();
    };
    const down = (event: PointerEvent) => {
      if (event.button !== 0 && event.button !== 1) return;
      drag.current = { x: event.clientX, y: event.clientY, pointer: event.pointerId };
    };
    const move = (event: PointerEvent) => {
      const held = drag.current;
      if (!held || held.pointer !== event.pointerId) return;
      const factor = target.current.z / Math.max(canvas.clientHeight, 1) * 0.75;
      target.current.x -= (event.clientX - held.x) * factor;
      target.current.y += (event.clientY - held.y) * factor;
      held.x = event.clientX;
      held.y = event.clientY;
      invalidate();
    };
    const up = () => { drag.current = null; };
    canvas.addEventListener("wheel", wheel, { passive: false });
    canvas.addEventListener("pointerdown", down);
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", up);
    window.addEventListener("pointercancel", up);
    return () => {
      canvas.removeEventListener("wheel", wheel);
      canvas.removeEventListener("pointerdown", down);
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", up);
      window.removeEventListener("pointercancel", up);
    };
  }, [gl, invalidate]);
  useFrame(() => {
    if (camera.position.distanceToSquared(target.current) < 0.00001) return;
    camera.position.lerp(target.current, reducedMotion ? 1 : 0.18);
    invalidate();
  });
  return null;
}

/** Pinch the handle to move the workspace itself; the headset camera is never moved. */
function SpatialPlacement({ children }: { children: ReactNode }) {
  const group = useRef<Group>(null);
  const session = useXR((state) => state.session);
  const gl = useThree((state) => state.gl);
  const held = useRef<{ source?: XRInputSource; start?: Vector3; origin?: Vector3; gain?: number } | null>(null);
  const recent = useRef<{ time: number; source: XRInputSource } | null>(null);
  const hand = useMemo(() => new Vector3(), []);
  const head = useMemo(() => new Vector3(), []);
  useEffect(() => {
    if (!session) return;
    const start = (event: XRInputSourceEvent) => {
      recent.current = { time: performance.now(), source: event.inputSource };
      if (held.current && !held.current.source) held.current.source = event.inputSource;
    };
    const end = (event: XRInputSourceEvent) => {
      if (held.current?.source === event.inputSource) held.current = null;
    };
    session.addEventListener("selectstart", start);
    session.addEventListener("selectend", end);
    return () => {
      held.current = null;
      session.removeEventListener("selectstart", start);
      session.removeEventListener("selectend", end);
    };
  }, [session]);
  useFrame((_state, _delta, frame) => {
    const reference = gl.xr.getReferenceSpace();
    const source = held.current?.source;
    if (!held.current || !group.current || !source?.gripSpace || !frame || !reference) return;
    const pose = frame.getPose(source.gripSpace, reference);
    if (!pose) return;
    hand.set(pose.transform.position.x, pose.transform.position.y, pose.transform.position.z);
    const xrCamera = gl.xr.getCamera();
    if (xrCamera.parent) hand.applyMatrix4(xrCamera.parent.matrixWorld);
    head.setFromMatrixPosition(xrCamera.matrixWorld);
    if (!held.current.start) {
      held.current.start = hand.clone();
      held.current.origin = group.current.position.clone();
      held.current.gain = gripGain(head, hand, group.current.position);
    }
    const translation = gripTranslation(held.current.start, hand, held.current.gain);
    if (held.current.origin) group.current.position.copy(held.current.origin).add(new Vector3(translation.x, translation.y, translation.z));
  });
  const grab = (event: ThreeEvent<PointerEvent>) => {
    event.stopPropagation();
    const source = recent.current && performance.now() - recent.current.time < 150 ? recent.current.source : undefined;
    held.current = { source };
  };
  return <group ref={group} position={[0, 1.45, -2.6]}>
    {children}
    <mesh userData={{ kind: "workspace-handle" }} position={[0, -0.9, 0.12]} onPointerDown={grab} onPointerUp={() => { held.current = null; }}>
      <capsuleGeometry args={[0.025, 0.32, 4, 8]} />
      <meshBasicMaterial color="#a4b9e6" />
    </mesh>
    <Label title="Pinch handle to reposition" detail="Your viewpoint stays still" color="#a4b9e6" position={[0, -1.05, 0]} width={0.75} />
  </group>;
}

function TaskInspector({ task, runs, evidence = [], artifacts = [] }: { task: Task; runs: Run[]; evidence?: Evidence[]; artifacts?: Artifact[] }) {
  const [page, setPage] = useState(0);
  const taskRuns = runs.filter((run) => run.project_id === task.project_id && run.task_id === task.id);
  const runIds = new Set(taskRuns.map((run) => run.id));
  const linked = (item: Evidence | Artifact) => item.project_id === task.project_id && (item.task_id === task.id || (item.run_id !== undefined && runIds.has(item.run_id)));
  const entries = [
    { title: task.title, detail: task.blocked_reason ?? task.description ?? `Source: ${task.source.system} · version ${task.version}` },
    ...(task.source_status ? [{ title: `Source reports ${task.source_status}`, detail: `Agent Room: ${task.status} · Review: ${task.review_state ?? "not requested"} · Approval: ${task.approval_state ?? "not required"}` }] : []),
    ...(task.source_observation ? [
      { title: "Latest source title", detail: task.source_observation.title },
      ...(task.source_observation.description ? [{ title: "Latest source report", detail: task.source_observation.description }] : []),
      { title: "Source report status", detail: `${task.source_observation.status} · ${task.source.system}` },
      ...(task.source_observed_at ? [{ title: "Source observed", detail: task.source_observed_at }] : []),
      { title: "Source report reference", detail: task.source_observation.external_url ?? task.source_observation.external_id },
    ] : []),
    ...taskRuns.map((run) => ({ title: `Worker · ${run.status}`, detail: run.summary ?? run.agent_id })),
    ...evidence.filter(linked).flatMap((item) => [
      { title: `${item.kind} · ${item.source.system}`, detail: item.summary },
      ...(item.source.external_url ? [{ title: "Evidence source reference", detail: item.source.external_url }] : []),
    ]),
    ...artifacts.filter(linked).flatMap((item) => [
      { title: "Artifact", detail: `${item.name} · ${item.media_type}` },
      { title: "Artifact reference", detail: item.uri },
    ]),
    ...(task.source.external_url ? [{ title: "Task source reference", detail: task.source.external_url }] : []),
  ].flatMap((entry) => textPages(entry.detail).map((detail) => ({ ...entry, detail })));
  const index = Math.min(page, entries.length - 1);
  const entry = entries[index];
  return <group position={[0, 0, 0]}>
    <Label title={entry?.title ?? task.title} detail={`${task.status} · ${index + 1}/${entries.length}`} color="#b9d4ff" position={[1.17, 0.1, 0.15]} width={0.92} />
    <Label title={entry?.detail ?? ""} detail="Recorded task and evidence" color="#a5b6d6" position={[1.17, -0.27, 0.15]} width={0.92} />
    {entries.length > 1 && <Label title="Next page" detail="Pinch to turn page" color="#9be7d2" position={[1.17, -0.61, 0.15]} width={0.65}
      onClick={() => { setPage((value) => (value + 1) % entries.length); }} />}
  </group>;
}

function RegionHub({ node, onSelect }: { node: SceneNode; onSelect: (node: SceneNode) => void }) {
  return <group position={node.position}>
    {node.action === "expand" && <mesh userData={{ kind: "portfolio-region", region: node.region, action: node.action }} onClick={(event) => { event.stopPropagation(); onSelect(node); }} rotation={[0.2, 0.4, 0]}>
      {node.region === "quiet" ? <torusGeometry args={[0.44, 0.04, 8, 40]} /> : node.region === "work" ? <ringGeometry args={[0.27, 0.5, 3]} /> : <octahedronGeometry args={[0.47, 0]} />}
      <meshStandardMaterial color={node.color} metalness={0.45} roughness={0.25} wireframe={node.region === "unknown"} />
    </mesh>}
    <Label title={node.title} detail={node.subtitle} color={node.color} position={[0, node.action === "expand" ? -1.15 : 0, 0.2]} width={node.action === "next" || node.action === "previous" ? 2.4 : 3.9} prominent onClick={() => onSelect(node)} />
  </group>;
}

function Participant({ node, onSelect }: { node: SceneNode; onSelect: (node: SceneNode) => void }) {
  return <mesh position={node.position} userData={{ kind: "agent-participant", agent_id: node.id }} onClick={(event) => { event.stopPropagation(); onSelect(node); }}>
    <capsuleGeometry args={[0.15, 0.25, 4, 12]} />
    <meshStandardMaterial color={node.color} metalness={0.4} roughness={0.25} />
  </mesh>;
}

/** Recorded participants and asks, readable independently of the project sculpture. */
function CoordinationSurface({ signals, human, compact = false, onSelect }: { signals: PortfolioSignal[]; human: boolean; compact?: boolean; onSelect?: (signal: PortfolioSignal) => void }) {
  const [page, setPage] = useState(0);
  const entries = signals.flatMap((signal) => {
    const attribution = `${signal.owner_name ?? signal.owner_id ?? "Owner not recorded"}${signal.recipient ? ` → ${signal.recipient_name ?? signal.recipient.id} (${signal.recipient.type})` : ""}`;
    return textPages(`${attribution}. ${signal.title}. ${signal.detail ?? ""}`).map((body) => ({ signal, body }));
  });
  const index = entries.length ? page % entries.length : 0;
  const entry = entries[index];
  const x = human ? 1.24 : -1.24;
  const width = human ? 0.78 : 1.02;
  const color = human ? "#cfbaff" : "#a5e9d6";
  return <group position={[0, compact ? 0.62 : 0, 0]} userData={{ kind: human ? "human-escalations" : "coordination-surface" }}>
    <Label title={human ? "Human escalations" : "Coordination"} detail={`${signals.length} authorized summary records`} color={color} position={[x, 0.51, 0.2]} width={width} />
    <Label title={entry?.body ?? (human ? "No explicit human escalation recorded" : "No agent work or request recorded")} detail={entry ? `${entry.signal.project_name} · ${entry.signal.status}` : "Coverage is shown separately"} color={color} position={[x, 0.1, 0.2]} width={width}
      onClick={entry && onSelect ? () => onSelect(entry.signal) : undefined} />
    {entry && !compact && <Label title={entry.signal.source_system ? `Source: ${entry.signal.source_system}` : "Recorded in Agent Room"} detail={entry.signal.updated_at} color="#b5c8de" position={[x, -0.29, 0.2]} width={width} />}
    {entries.length > 1 && <Label title="Next coordination page" detail={`${index + 1}/${entries.length}`} color={color} position={[x, compact ? 0.85 : -0.65, 0.2]} width={0.7} onClick={() => setPage((value) => value + 1)} />}
  </group>;
}

function RequestInspector({ request, agents = [], signals = [] }: { request: CoordinationRequest; agents?: Agent[]; signals?: PortfolioSignal[] }) {
  const [page, setPage] = useState(0);
  const name = (id: string) => agents.find((item) => item.id === id)?.display_name ?? signals.find((item) => item.owner_id === id)?.owner_name ?? signals.find((item) => item.recipient?.id === id)?.recipient_name ?? id;
  const pages = [
    `Request: ${request.title}`,
    `From ${name(request.owner_id)} (${request.owner_type}) to ${name(request.recipient.id)} (${request.recipient.type}). ${request.kind} · ${request.status}`,
    request.body,
    ...(request.escalation_reason ? [`Escalation reason: ${request.escalation_reason}`] : []),
    ...(request.response ? [`Response by ${request.response.actor_id}: ${request.response.body}`] : []),
    ...(request.resolution ? [`Closed by ${name(request.resolution.actor_id)}: ${request.resolution.note}. ${request.resolution.closed_at}`] : []),
    ...request.evidence_ids.map((id) => `Evidence reference: ${id}`),
    ...(request.response?.evidence_ids ?? []).map((id) => `Response evidence reference: ${id}`),
    `Request recorded ${request.updated_at} · version ${request.version}`,
  ].flatMap((value) => textPages(value));
  return <group userData={{ kind: "request-inspector", request_id: request.id }}>
    <Label title="Coordination request" detail={`${request.status} · ${page + 1}/${pages.length}`} color="#c7b4ef" position={[1.17, 0.1, 0.15]} width={0.92} />
    <Label title={pages[page] ?? ""} detail="Respond in the project inspector" color="#b9c9de" position={[1.17, -0.27, 0.15]} width={0.92} />
    <Label title="Next request page" detail="Pinch to turn page" color="#9be7d2" position={[1.17, -0.61, 0.15]} width={0.65} onClick={() => setPage((value) => (value + 1) % pages.length)} />
  </group>;
}

function SceneContent(props: SceneContentProps) {
  const { projection, immersive, animate, reducedMotion, selectedTaskId, resetToken } = props;
  const [hovered, setHovered] = useState<string | null>(null);
  const selected = selectedTaskId ?? props.activeProjectId;
  const select = (node: SceneNode) => {
    if (node.kind === "region") {
      if (node.action === "next" || node.action === "previous") props.onRegionPageChange?.((projection.page ?? 0) + (node.action === "next" ? 1 : -1));
      else props.onRegionSelect?.(node.action === "collapse" ? null : node.region ?? null);
    } else if (node.kind === "project") props.onProjectSelect(node.id);
    else if (node.kind === "task") props.onTaskSelect(node.id);
    else {
      const signal = props.signals?.find((item) => item.id === node.id && item.project_id === props.activeProjectId);
      if (signal) props.onSignalSelect?.(signal);
    }
  };
  const selectedNode = projection.nodes.find((node) => node.id === selected);
  const selectedTask = props.tasks.find((task) => task.id === selectedTaskId);
  const selectedRequest = props.requests?.find((request) => request.id === props.selectedRequestId);
  const labels = visibleLabels(projection.nodes, selected, hovered);
  const sculpture = <group>
    <Observatory />
    <DependencyThreads projection={projection} selected={selectedTaskId} />
    <Constellation nodes={projection.nodes.filter((node) => node.kind === "project" || node.kind === "task")} selected={selected} onSelect={select} onHover={setHovered} />
    {projection.nodes.filter((node) => node.kind === "region").map((node) => <RegionHub key={node.id} node={node} onSelect={select} />)}
    {projection.nodes.filter((node) => node.kind === "agent").map((node) => <Participant key={node.id} node={node} onSelect={select} />)}
    {projection.nodes.filter((node) => node.active).map((node) => <SignalRing key={node.id} node={node} animate={animate && !reducedMotion} />)}
    {selectedNode && <mesh position={selectedNode.position} raycast={() => undefined}>
      <torusGeometry args={[selectedNode.kind === "project" ? 0.5 : 0.34, 0.008, 6, 48]} />
      <meshBasicMaterial color="#e5edff" transparent opacity={0.8} toneMapped={false} />
    </mesh>}
    {labels.filter((node) => node.kind !== "region").map((node) => <Label key={node.id} title={node.kind === "agent" ? `Agent · ${node.title}` : node.title} detail={node.attention ? `${node.subtitle} · ${node.attention} attention` : node.subtitle}
      color={node.color} position={[node.position[0], node.position[1] - (node.kind === "project" ? 0.85 : 0.7), node.position[2] + 0.12]}
      width={2.55} prominent onClick={() => { select(node); }} />)}
  </group>;
  if (!immersive) return <>
    <DesktopNavigation nodes={projection.nodes} resetToken={resetToken} reducedMotion={reducedMotion} />
    {sculpture}
  </>;
  return <SpatialPlacement>
    <group scale={Math.min(0.17, 1.5 / projection.radius)}>{sculpture}</group>
    <Label title="Exit immersive" detail="Return to desktop" color="#bbceef" position={[0.48, 0.9, 0.1]} width={0.65} onClick={props.onExit} />
    {props.onBack && <Label title="All projects" detail="Return to portfolio" color="#b3d9ff" position={[-0.48, 0.9, 0.1]} width={0.65} onClick={props.onBack} />}
    {projection.unresolvedDependencies > 0 && <Label title="Dependency details unavailable" detail={`${projection.unresolvedDependencies} declared links`} color="#ffca92" position={[-0.48, 0.55, 0.1]} width={0.75} />}
    {projection.cycleAffectedTaskIds.length > 0 && <Label title="Tasks in / downstream of cycle" detail={`${projection.cycleAffectedTaskIds.length} tasks`} color="#ffca92" position={[0.48, 0.55, 0.1]} width={0.75} />}
    <CoordinationSurface signals={(props.signals ?? []).filter((item) => !item.human_escalation)} human={false} onSelect={props.onSignalSelect} />
    <CoordinationSurface signals={(props.signals ?? []).filter((item) => item.human_escalation)} human compact={Boolean(selectedTask || selectedRequest)} onSelect={props.onSignalSelect} />
    {selectedTask ? <TaskInspector key={selectedTask.id} task={selectedTask} runs={props.runs} evidence={props.evidence} artifacts={props.artifacts} /> : selectedRequest ? <RequestInspector key={`${selectedRequest.id}:${selectedRequest.version}`} request={selectedRequest} agents={props.agents} signals={props.signals} /> : null}
  </SpatialPlacement>;
}

function ImmersiveContent(props: Omit<SceneContentProps, "immersive">) {
  const active = useXR((state) => state.session !== undefined);
  return active ? <SceneContent {...props} immersive /> : null;
}

/** Shared data/geometry with separate renderers: Fiber's XR loop currently requires WebGLManager. */
export function SceneCanvas(props: SceneCanvasProps) {
  const projection = useMemo(() => buildWorkspaceScene(props.projects, props.activeProjectId, props.tasks, props.runs, props.attention,
    { portfolio: props.portfolio ?? {}, region: props.region, page: props.page, now: props.now, agents: props.agents }),
    [props.projects, props.activeProjectId, props.tasks, props.runs, props.attention, props.portfolio, props.region, props.page, props.now, props.agents]);
  const [graphics, setGraphics] = useState<"checking" | "available" | "failed">("checking");
  const [renderer, setRenderer] = useState("Starting renderer");
  const [forceGL, setForceGL] = useState(false);
  const [xrSupported, setXRSupported] = useState(false);
  const [xrReady, setXRReady] = useState(false);
  const [immersive, setImmersive] = useState(false);
  const [entering, setEntering] = useState(false);
  const [error, setError] = useState("");
  const [resetToken, setResetToken] = useState(0);
  const [visible, setVisible] = useState(true);
  const wrapper = useRef<HTMLDivElement>(null);
  const xrManager = useRef<WebXRManager | null>(null);
  const xrSession = useRef<XRSession | null>(null);
  const mounted = useRef(true);
  const rendererCleanups = useRef<(() => void)[]>([]);
  const [store, setStore] = useState<ReturnType<typeof createXRStore> | null>(null);
  useEffect(() => {
    mounted.current = true;
    const graphicSupport = typeof WebGL2RenderingContext !== "undefined" || "gpu" in navigator;
    setGraphics(graphicSupport ? "available" : "failed");
    if (navigator.xr && window.isSecureContext) {
      void navigator.xr.isSessionSupported("immersive-vr").then((supported) => {
        if (mounted.current) setXRSupported(supported);
      }).catch(() => { if (mounted.current) setXRSupported(false); });
    }
    return () => { mounted.current = false; };
  }, []);
  useEffect(() => {
    // createXRStore owns DOM/listeners; allocate in an effect so StrictMode can clean each instance.
    const current = createXRStore({
      emulate: false, offerSession: false, enterGrantedSession: false, domOverlay: false,
      handTracking: false, gaze: false, controller: { model: false }, hand: { model: false },
    });
    setStore(current);
    const unsubscribe = current.subscribe((state) => {
      const session = state.session;
      xrSession.current = session ?? null;
      if (mounted.current) setImmersive(session !== undefined);
    });
    return () => {
      unsubscribe();
      const session = xrSession.current;
      if (session) void session.end().catch(() => undefined);
      current.destroy();
      rendererCleanups.current.forEach((cleanup) => { cleanup(); });
      rendererCleanups.current = [];
    };
  }, []);
  useEffect(() => {
    let inView = true;
    const update = () => { setVisible(inView && document.visibilityState !== "hidden"); };
    const observer = typeof IntersectionObserver === "undefined" ? null : new IntersectionObserver((entries) => {
      inView = entries.some((entry) => entry.isIntersecting);
      update();
    });
    if (wrapper.current) observer?.observe(wrapper.current);
    document.addEventListener("visibilitychange", update);
    update();
    return () => { observer?.disconnect(); document.removeEventListener("visibilitychange", update); };
  }, []);
  const failGraphics = useCallback(() => { setGraphics("failed"); setRenderer("Accessible workspace"); }, []);
  const createDesktopRenderer = useCallback(async (defaults: { canvas: EventTarget }) => {
    if (!forceGL && "gpu" in navigator) {
      try {
        const { WebGPURenderer } = await import("three/webgpu");
        const gpu = new WebGPURenderer({ canvas: defaults.canvas as HTMLCanvasElement, antialias: true, alpha: false });
        await gpu.init();
        const backend = gpu.backend as { isWebGPUBackend?: boolean; device?: { lost: Promise<{ reason: string }> } };
        if (mounted.current) setRenderer(backend.isWebGPUBackend ? "WebGPU" : "WebGL 2");
        if (backend.device) void backend.device.lost.then(() => { if (mounted.current) { setForceGL(true); setError("The GPU device was lost. Switched to WebGL 2; your selection is preserved."); } });
        return gpu;
      } catch {
        if (mounted.current) setError("WebGPU is unavailable. Using WebGL 2.");
      }
    }
    try {
      const gl = new WebGLRenderer({ canvas: defaults.canvas as HTMLCanvasElement, antialias: true, alpha: false, powerPreference: "high-performance" });
      if (mounted.current) setRenderer("WebGL 2");
      return gl;
    } catch (error) {
      if (mounted.current) failGraphics();
      throw error;
    }
  }, [forceGL, failGraphics]);
  const monitorRenderer = useCallback((state: RootState) => {
    const canvas = state.gl.domElement;
    const lost = (event: Event) => { event.preventDefault(); failGraphics(); setError("The graphics context was lost. Use the project and task controls below to continue."); };
    canvas.addEventListener("webglcontextlost", lost);
    rendererCleanups.current.push(() => { canvas.removeEventListener("webglcontextlost", lost); });
    state.gl.setClearColor("#060c1a");
  }, [failGraphics]);
  const exit = useCallback(() => {
    const session = xrSession.current;
    if (session) void session.end().catch(() => { setError("Use the headset's system control to leave the session."); });
  }, []);
  const enter = () => {
    const manager = xrManager.current;
    if (!manager || !navigator.xr || entering) return;
    setError("");
    setEntering(true);
    // No await before requestImmersiveSession: preserve transient activation on Safari.
    void requestImmersiveSession(manager, (mode, init) => navigator.xr!.requestSession(mode, init)).then((session) => {
      if (!mounted.current) { void session.end().catch(() => undefined); return; }
      xrSession.current = session;
    }).catch((reason: unknown) => { if (mounted.current) setError(sessionError(reason)); }).finally(() => { if (mounted.current) setEntering(false); });
  };
  const contentProps = { ...props, projection, resetToken, animate: visible && !immersive, onExit: exit };
  const graphNotice = [
    ...(projection.unresolvedDependencies > 0 ? [`Dependency details unavailable: ${projection.unresolvedDependencies}.`] : []),
    ...(projection.cycleAffectedTaskIds.length > 0 ? [`Tasks in or downstream of a dependency cycle: ${projection.cycleAffectedTaskIds.length}.`] : []),
  ].join(" ");
  return <div ref={wrapper} className="workspace-scene" data-renderer={renderer}>
    {graphics === "available" && <GraphicsBoundary onFailure={failGraphics}>
      <div className="workspace-scene__canvas" aria-hidden="true">
        <Canvas key={forceGL ? "webgl" : "auto"} frameloop="demand" dpr={[1, 1.5]} gl={createDesktopRenderer}
          camera={{ position: [0, 0, 20], fov: 42, near: 0.05, far: 180 }} onCreated={monitorRenderer}>
          <SceneContent {...contentProps} immersive={false} />
        </Canvas>
      </div>
    </GraphicsBoundary>}
    {graphics === "checking" && <div className="workspace-scene__fallback">Preparing your observatory…</div>}
    {graphics === "failed" && <div className="workspace-scene__fallback">
      <span aria-hidden="true">✦</span><p>Your workspace is ready in the project and task controls below.</p>
      <p>Spatial rendering requires WebGPU or WebGL 2. Your data and evidence remain available.</p>
    </div>}
    <div className="workspace-scene__toolbar">
      <span className="workspace-scene__status">{immersive ? "WebXR · WebGL 2" : renderer}</span>
      {graphics === "available" && <button type="button" onClick={() => { setResetToken((value) => value + 1); }}>Recenter view</button>}
      {xrSupported && <button type="button" disabled={!xrReady || entering} onClick={immersive ? exit : enter}>
        {immersive ? "Exit immersive" : entering ? "Entering…" : xrReady ? "Enter immersive" : "Preparing headset…"}
      </button>}
      {!xrSupported && <span className="workspace-scene__xr-note">Headset mode appears in a compatible secure browser</span>}
    </div>
    {(error || graphNotice) && <div role="status" className="workspace-scene__message">{error && <p>{error}</p>}{graphNotice && <p>{graphNotice}</p>}</div>}
    {graphics === "available" && <p className="workspace-scene__hint">Drag to explore · Scroll to move closer · Select a light to unfold its work</p>}
    {xrSupported && store && <GraphicsBoundary onFailure={() => { setXRReady(false); setError("Headset rendering is unavailable. The desktop workspace remains ready."); }}>
      <div style={invisibleStage} aria-hidden="true">
        <Canvas frameloop="demand" gl={{ antialias: true, alpha: false, powerPreference: "high-performance" }}
          onCreated={(state) => { xrManager.current = state.gl.xr; state.gl.setClearColor("#060c1a"); setXRReady(true); }}>
          <XR store={store}><ImmersiveContent {...contentProps} animate={immersive} /></XR>
        </Canvas>
      </div>
    </GraphicsBoundary>}
  </div>;
}
