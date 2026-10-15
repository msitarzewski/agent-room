import type { WebXRManager } from "three";

export interface GripPoint { x: number; y: number; z: number }

/** Direct hand displacement, rather than the gaze ray, drives spatial dragging. */
export function gripTranslation(start: GripPoint, current: GripPoint, gain = 1): GripPoint {
  const factor = Math.max(1, Math.min(3, gain));
  return { x: (current.x - start.x) * factor, y: (current.y - start.y) * factor, z: (current.z - start.z) * factor };
}

export function gripGain(head: GripPoint, hand: GripPoint, object: GripPoint): number {
  const distance = (a: GripPoint, b: GripPoint) => Math.hypot(a.x - b.x, a.y - b.y, a.z - b.z);
  const reach = distance(head, hand);
  return reach > 0.05 ? Math.min(3, Math.max(1, distance(head, object) / reach)) : 1;
}

/** Call synchronously inside the click: Safari's activation must reach requestSession. */
export function requestImmersiveSession(
  manager: Pick<WebXRManager, "setSession">,
  request: (mode: XRSessionMode, options: XRSessionInit) => Promise<XRSession>,
): Promise<XRSession> {
  const sessionRequest = request("immersive-vr", { requiredFeatures: ["local-floor"], optionalFeatures: ["layers"] });
  return sessionRequest.then(async (session) => {
    try {
      await manager.setSession(session);
      return session;
    } catch (error) {
      await session.end().catch(() => undefined);
      throw error;
    }
  });
}

export function sessionError(error: unknown): string {
  const name = error instanceof Error || error instanceof DOMException ? error.name : "";
  if (name === "NotAllowedError") return "Headset entry was cancelled or permission was declined. You can try again.";
  if (name === "NotSupportedError") return "This browser cannot start the requested headset session. The desktop workspace remains available.";
  return "The headset session could not start. The desktop workspace remains available; try entering again.";
}
