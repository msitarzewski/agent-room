import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { vi } from "vitest";
import {
  ActionFeedback,
  Freshness,
  PageHeader,
  RelativeTime,
  SourceFreshness,
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
    expect(screen.getByText("45 seconds ago")).toBeInTheDocument();
    expect(screen.getByText("not-a-date")).toBeInTheDocument();
    expect(screen.getByText("Never")).toBeInTheDocument();
    vi.useRealTimers();
  });

  it("updates an idle elapsed label and releases its timer when the value is removed", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-10-14T12:00:00Z"));
    const view = render(<RelativeTime value="2026-10-14T11:59:15Z" />);
    expect(screen.getByText("45 seconds ago")).toBeInTheDocument();
    act(() => { vi.advanceTimersByTime(15_000); });
    expect(screen.getByText("1 minute ago")).toBeInTheDocument();
    act(() => { vi.advanceTimersByTime(60_000); });
    expect(screen.getByText("2 minutes ago")).toBeInTheDocument();
    view.rerender(<RelativeTime value={null} />);
    expect(vi.getTimerCount()).toBe(0);
    view.rerender(<RelativeTime value="not-a-date" />);
    expect(vi.getTimerCount()).toBe(0);
    view.unmount();
    vi.useRealTimers();
  });

  it("separates collection age from queued delivery receipt and reports successful zero-task polls", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-10-14T12:00:00Z"));
    const source = { system: "sentry", last_seen: "2026-10-14T12:00:00Z", last_received_at: "2026-10-14T12:00:00Z",
      last_collected_at: "2026-10-13T16:00:00Z", last_successful_collection_at: "2026-10-13T16:00:00Z", scan_status: "complete" as const,
      project_observation: { external_id: "repo-empty", name: "Observed source name", path: "Software/empty", status: "active", collected_at: "2026-10-13T16:00:00Z", scan_status: "complete" as const, task_count: 0 } };
    const view = render(<SourceFreshness source={source} />);
    expect(screen.getByText(/Source collected/)).toHaveTextContent("20 hours ago");
    expect(screen.getByText(/Agent Room received/)).toHaveTextContent("now");
    expect(screen.getByText("Complete collection · 0 tasks observed")).toBeInTheDocument();
    expect(screen.getByText("Source project: Observed source name · active")).toBeInTheDocument();
    view.rerender(<SourceFreshness source={{ ...source, last_collected_at: null, scan_status: "partial", project_observation: { ...source.project_observation, scan_status: "partial", scan_error: "One task file was unreadable." } }} />);
    expect(screen.getByText(/Last complete collection/)).toHaveTextContent("20 hours ago");
    expect(screen.getByText(/One task file was unreadable/)).toBeInTheDocument();
    view.rerender(<SourceFreshness source={{ system: "legacy", last_seen: source.last_seen }} compact />);
    expect(screen.getByText("Collection time not reported")).toBeInTheDocument();
    expect(screen.getByText("Collection completeness unknown")).toBeInTheDocument();
    view.rerender(<SourceFreshness source={{ ...source, scan_status: "unreadable", project_observation: { ...source.project_observation, reason: "Repository inaccessible." } }} />);
    expect(screen.getByText("Collection unreadable · task count unavailable")).toBeInTheDocument();
    expect(screen.getByText(/Repository inaccessible/)).toBeInTheDocument();
    view.unmount();
    expect(vi.getTimerCount()).toBe(0);
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
