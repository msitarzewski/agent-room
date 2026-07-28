import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { vi } from "vitest";
import {
  ActionFeedback,
  Freshness,
  PageHeader,
  RelativeTime,
  StatePanel,
  StatusPill,
  formatBytes,
  formatNumber,
  hasCapability,
} from "../components/Common";

describe("shared interface primitives", () => {
  it.each([
    ["loading", "Loading current state"],
    ["denied", "Access denied"],
    ["error", "Unable to load this view"],
    ["empty", "Nothing here"],
  ] as const)("renders the %s collection state", async (state, expected) => {
    const retry = vi.fn();
    const user = userEvent.setup();
    render(<StatePanel state={state} error="Network detail" emptyTitle="Nothing here" emptyBody="No records." onRetry={retry}><div>Ready</div></StatePanel>);
    expect(screen.getByText(expected)).toBeInTheDocument();
    if (state === "error") {
      await user.click(screen.getByRole("button", { name: "Try again" }));
      expect(retry).toHaveBeenCalledOnce();
    }
  });

  it("renders ready content and composable presentation helpers", () => {
    render(
      <StatePanel state="ready" error={null} emptyTitle="" emptyBody="" onRetry={vi.fn()}>
        <PageHeader eyebrow="Scope" title="Ready" description="Canonical data" actions={<StatusPill value="in_progress" />} />
        <ActionFeedback message="Saved" error={null} />
        <ActionFeedback message={null} error="Rejected" />
      </StatePanel>,
    );
    expect(screen.getByRole("heading", { name: "Ready" })).toBeInTheDocument();
    expect(screen.getByText("in progress")).toBeInTheDocument();
    expect(screen.getByText("Saved")).toBeInTheDocument();
    expect(screen.getByText("Rejected")).toHaveClass("action-feedback--error");
  });

  it("detects stale data and formats valid, invalid, and missing time", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-07-27T12:00:00Z"));
    render(<>
      <Freshness loadedAt="2026-07-27T11:58:00Z" />
      <RelativeTime value="2026-07-27T11:59:30Z" />
      <RelativeTime value="not-a-date" />
      <RelativeTime value={null} />
    </>);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(15_000);
    });
    expect(screen.getByText(/may be stale/)).toBeInTheDocument();
    expect(screen.getByText("30 seconds ago")).toBeInTheDocument();
    expect(screen.getByText("not-a-date")).toBeInTheDocument();
    expect(screen.getByText("Never")).toBeInTheDocument();
    vi.useRealTimers();
  });

  it("formats values and enforces wildcard or exact capabilities", () => {
    expect(formatBytes(20)).toBe("20 B");
    expect(formatBytes(2048)).toBe("2.0 KB");
    expect(formatBytes(20 * 1024 * 1024)).toBe("20 MB");
    expect(formatNumber(1200)).toMatch(/1.?200/);
    expect(hasCapability(["run:pause"], "run:pause")).toBe(true);
    expect(hasCapability(["*"], "run:cancel")).toBe(true);
    expect(hasCapability([], "run:cancel")).toBe(false);
  });
});
