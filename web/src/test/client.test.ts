import { vi } from "vitest";
import { api, ApiError } from "../api/client";
import { approval, attention, claim, run, task } from "./fixtures";

function response(body: unknown, init?: ResponseInit): Response {
  return body === undefined
    ? new Response(null, init)
    : Response.json(body, init);
}

function callAt(fetch: ReturnType<typeof vi.fn>, index = 0): [string, RequestInit] {
  return fetch.mock.calls[index] as unknown as [string, RequestInit];
}

function jsonBody(init: RequestInit): unknown {
  if (typeof init.body !== "string") throw new Error("Expected a JSON string request body.");
  return JSON.parse(init.body) as unknown;
}

describe("canonical API client contract", () => {
  it("uses same-origin credentials and the canonical collection query parameters", async () => {
    const fetch = vi.fn()
      .mockResolvedValueOnce(response({ items: [] }))
      .mockResolvedValueOnce(response({ items: [], next_cursor: 42 }));
    vi.stubGlobal("fetch", fetch);
    const abort = new AbortController();

    await api.tasks("project with spaces", abort.signal);
    await api.events("project_alpha", 41, abort.signal);

    expect(fetch).toHaveBeenNthCalledWith(
      1,
      "/api/v1/tasks?project_id=project+with+spaces",
      expect.objectContaining({ credentials: "include", signal: abort.signal }),
    );
    expect(fetch).toHaveBeenNthCalledWith(
      2,
      "/api/v1/events?project_id=project_alpha&cursor=41",
      expect.objectContaining({ credentials: "include", signal: abort.signal }),
    );
    const [, firstInit] = callAt(fetch);
    expect(new Headers(firstInit.headers).get("Accept")).toBe("application/json");
  });

  it("normalizes absent project collections to an empty authorized-project list", async () => {
    const fetch = vi.fn()
      .mockResolvedValueOnce(response({ items: null }))
      .mockResolvedValueOnce(response(null));
    vi.stubGlobal("fetch", fetch);

    await expect(api.projects()).resolves.toEqual({ items: [] });
    await expect(api.projects()).resolves.toEqual({ items: [] });
  });

  it.each([
    [
      "run action",
      () => api.runAction({ ...run, id: "run/unsafe" }, "redirect", "Investigate root cause."),
      "/api/v1/runs/run%2Funsafe/actions?project_id=project_alpha",
      { action: "redirect", message: "Investigate root cause." },
      "1",
    ],
    [
      "task transition",
      () => api.transitionTask({ ...task, id: "task/unsafe" }, "blocked", "Dependency unavailable."),
      "/api/v1/tasks/task%2Funsafe/transition?project_id=project_alpha",
      { status: "blocked", reason: "Dependency unavailable." },
      "1",
    ],
    [
      "attention mutation",
      () => api.attentionAction({ ...attention, id: "attention/unsafe" }, "resolve"),
      "/api/v1/attention/attention%2Funsafe/resolve?project_id=project_alpha",
      {},
      "1",
    ],
    [
      "claim review",
      () => api.reviewClaim({ ...claim, id: "claim/unsafe" }, "accepted"),
      "/api/v1/claims/claim%2Funsafe/review?project_id=project_alpha",
      { decision: "accepted" },
      "1",
    ],
    [
      "approval decision",
      () => api.decideApproval({ ...approval, id: "approval/unsafe" }, "rejected", "Digest mismatch."),
      "/api/v1/approvals/approval%2Funsafe/decision?project_id=project_alpha",
      { decision: "rejected", note: "Digest mismatch." },
      "1",
    ],
  ])("sends %s with CSRF, idempotency, version, and an encoded resource id", async (
    _name,
    invoke,
    expectedPath,
    expectedBody,
    expectedVersion,
  ) => {
    const fetch = vi.fn().mockResolvedValue(response({ id: "updated" }));
    vi.stubGlobal("fetch", fetch);
    api.setCsrfToken("csrf-token");

    await invoke();

    const [path, init] = callAt(fetch);
    const headers = new Headers(init.headers);
    expect(path).toBe(expectedPath);
    expect(init.method).toBe("POST");
    expect(jsonBody(init)).toEqual(expectedBody);
    expect(headers.get("Content-Type")).toBe("application/json");
    expect(headers.get("X-CSRF-Token")).toBe("csrf-token");
    expect(headers.get("Idempotency-Key")).toMatch(/^[0-9a-f-]{36}$/i);
    expect(headers.get("If-Match")).toBe(expectedVersion);
  });

  it("requests the exact cancellable command and never invents an If-Match version", async () => {
    const fetch = vi.fn().mockResolvedValue(response({ id: "approval_new" }, { status: 201 }));
    vi.stubGlobal("fetch", fetch);
    api.setCsrfToken("csrf-token");

    await api.requestRunApproval(run, "cancel", "Unsafe output.", 600);

    const [path, init] = callAt(fetch);
    const headers = new Headers(init.headers);
    expect(path).toBe("/api/v1/approvals?project_id=project_alpha");
    expect(jsonBody(init)).toEqual({
      run_id: "run_1",
      action: "cancel",
      message: "Unsafe output.",
      expected_target_version: 1,
      expires_in_seconds: 600,
    });
    expect(headers.has("If-Match")).toBe(false);
  });

  it("supports a 204 logout and clears transport assumptions about a response body", async () => {
    const fetch = vi.fn().mockResolvedValue(response(undefined, { status: 204 }));
    vi.stubGlobal("fetch", fetch);
    api.setCsrfToken("csrf-token");

    await expect(api.logout()).resolves.toBeUndefined();
    const [, init] = callAt(fetch);
    expect(new Headers(init.headers).get("X-CSRF-Token")).toBe("csrf-token");
  });

  it("normalizes omitted brief collections before they reach the renderer", async () => {
    const fetch = vi.fn().mockResolvedValue(response({
      project_id: "project_alpha",
      from_cursor: 0,
      through_cursor: 0,
      reviewed_cursor: 0,
      generated_at: "2026-07-27T12:00:00Z",
    }));
    vi.stubGlobal("fetch", fetch);

    await expect(api.brief("project_alpha")).resolves.toMatchObject({
      event_counts: {},
      events: [],
      open_attention: [],
      pending_approvals: [],
      recommended_actions: [],
    });
  });

  it("preserves populated brief fields and requests an explicit replay cursor", async () => {
    const fetch = vi.fn().mockResolvedValue(response({
      project_id: "project_alpha",
      from_cursor: 42,
      through_cursor: 43,
      reviewed_cursor: 41,
      event_counts: { "task.completed": 1 },
      events: [{ cursor: 43 }],
      open_attention: [{ id: "attention_1" }],
      pending_approvals: [{ id: "approval_1" }],
      recommended_actions: ["Review pending approvals"],
      generated_at: "2026-07-27T12:00:00Z",
    }));
    vi.stubGlobal("fetch", fetch);

    const brief = await api.brief("project_alpha", 42);

    expect(callAt(fetch)[0]).toBe("/api/v1/brief?project_id=project_alpha&after=42");
    expect(brief.event_counts).toEqual({ "task.completed": 1 });
    expect(brief.events).toHaveLength(1);
    expect(brief.open_attention).toHaveLength(1);
    expect(brief.pending_approvals).toHaveLength(1);
    expect(brief.recommended_actions).toEqual(["Review pending approvals"]);
  });

  it("preserves problem details and falls back safely for a non-JSON failure", async () => {
    const fetch = vi.fn()
      .mockResolvedValueOnce(response({
        title: "Conflict",
        detail: "Version mismatch.",
        status: 409,
        code: "conflict",
      }, { status: 409 }))
      .mockResolvedValueOnce(new Response("<html>unavailable</html>", {
        status: 502,
        statusText: "Bad Gateway",
        headers: { "Content-Type": "text/html" },
      }));
    vi.stubGlobal("fetch", fetch);

    try {
      await api.tasks("project_alpha");
      throw new Error("Expected the conflict request to fail.");
    } catch (caught) {
      expect(caught).toBeInstanceOf(ApiError);
      if (!(caught instanceof ApiError)) throw caught;
      expect(caught.message).toBe("Version mismatch.");
      expect(caught.status).toBe(409);
      expect(caught.problem.code).toBe("conflict");
    }
    await expect(api.tasks("project_alpha")).rejects.toMatchObject({
      name: "ApiError",
      message: "Bad Gateway",
      status: 502,
    });
  });

  it("encodes the OIDC return target without accepting a second query field", () => {
    expect(api.loginUrl("/tasks?filter=review&owner=me")).toBe(
      "/api/v1/auth/login?return_to=%2Ftasks%3Ffilter%3Dreview%26owner%3Dme",
    );
  });
});
