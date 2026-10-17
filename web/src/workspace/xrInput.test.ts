import { describe, expect, it, vi } from "vitest";
import type { WebXRManager } from "three";
import { gripGain, gripTranslation, requestImmersiveSession, sessionError } from "./xrInput";

describe("Vision Pro hand displacement", () => {
  it("moves along the hand's displacement with a bounded comfortable gain", () => {
    expect(gripTranslation({ x: 1, y: 2, z: 3 }, { x: 2, y: 0, z: 4 }, 2)).toEqual({ x: 2, y: -4, z: 2 });
    expect(gripTranslation({ x: 0, y: 0, z: 0 }, { x: 1, y: 1, z: 1 }, 20)).toEqual({ x: 3, y: 3, z: 3 });
    expect(gripTranslation({ x: 0, y: 0, z: 0 }, { x: 1, y: 1, z: 1 }, 0)).toEqual({ x: 1, y: 1, z: 1 });
    expect(gripGain({ x: 0, y: 0, z: 0 }, { x: 0, y: 0, z: 0 }, { x: 0, y: 0, z: -2 })).toBe(1);
    expect(gripGain({ x: 0, y: 0, z: 0 }, { x: 0, y: 0, z: -0.5 }, { x: 0, y: 0, z: -2 })).toBe(3);
    expect(gripGain({ x: 0, y: 0, z: 0 }, { x: 0, y: 0, z: -2 }, { x: 0, y: 0, z: -1 })).toBe(1);
  });
});

describe("activation-safe immersive sessions", () => {
  it("requests the session synchronously, then binds the renderer", async () => {
    const session = { end: vi.fn() } as unknown as XRSession;
    const request = vi.fn().mockResolvedValue(session);
    const manager = { setSession: vi.fn().mockResolvedValue(undefined) };
    const promise = requestImmersiveSession(manager, request);
    expect(request).toHaveBeenCalledWith("immersive-vr", { requiredFeatures: ["local-floor"], optionalFeatures: ["layers"] });
    expect(manager.setSession).not.toHaveBeenCalled();
    expect(await promise).toBe(session);
    expect(manager.setSession).toHaveBeenCalledWith(session);
  });

  it("ends the acquired session when the renderer cannot bind", async () => {
    const end = vi.fn().mockRejectedValue(new Error("already ended"));
    const session = { end } as unknown as XRSession;
    const error = new Error("bind failed");
    const manager = { setSession: vi.fn().mockRejectedValue(error) };
    await expect(requestImmersiveSession(manager as Pick<WebXRManager, "setSession">, vi.fn().mockResolvedValue(session))).rejects.toBe(error);
    expect(end).toHaveBeenCalledOnce();
  });

  it("leaves the renderer untouched when entry is cancelled and supplies recoverable errors", async () => {
    const manager = { setSession: vi.fn() };
    const error = new DOMException("declined", "NotAllowedError");
    await expect(requestImmersiveSession(manager as Pick<WebXRManager, "setSession">, vi.fn().mockRejectedValue(error))).rejects.toBe(error);
    expect(manager.setSession).not.toHaveBeenCalled();
    expect(sessionError(error)).toContain("cancelled");
    expect(sessionError(new DOMException("unsupported", "NotSupportedError"))).toContain("cannot start");
    expect(sessionError({ detail: "opaque" })).toContain("could not start");
  });
});
