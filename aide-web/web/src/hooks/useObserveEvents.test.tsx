// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  useObserveEvents,
  type UseObserveEventsOptions,
  type UseObserveEventsResult,
} from "./useObserveEvents";
import type { ObserveEventItem } from "../lib/types";

const { list } = vi.hoisted(() => ({ list: vi.fn() }));
vi.mock("@/lib/api", () => ({
  api: {
    listObserveEvents: list,
    observeWatchUrl: (project: string, filters: object) =>
      `${project}?${JSON.stringify(filters)}`,
  },
}));

class Stream {
  static CLOSED = 2;
  static instances: Stream[] = [];
  readyState = 1;
  onopen: (() => void) | null = null;
  onmessage: ((event: { data: string }) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  constructor(public url: string) {
    Stream.instances.push(this);
  }
  close() {
    this.readyState = Stream.CLOSED;
  }
  emit(event: ObserveEventItem) {
    this.onmessage?.({ data: JSON.stringify(event) });
  }
}

let root: Root;
let container: HTMLDivElement;
let current: UseObserveEventsResult;
let requests: ((events: ObserveEventItem[]) => void)[];
const event = (id: string, name = "Read"): ObserveEventItem => ({
  id,
  name,
  kind: "tool_call",
  timestamp: "2026-09-15T12:00:00Z",
});
function Probe(props: UseObserveEventsOptions) {
  current = useObserveEvents(props);
  return null;
}
async function render(project = "a", name?: string) {
  await act(async () =>
    root.render(<Probe project={project} filters={{ name }} maxEvents={3} />),
  );
}
async function live(enabled: boolean) {
  await act(async () => current.setLiveTail(enabled));
}
async function tick() {
  await act(async () => vi.advanceTimersByTime(100));
}
const stream = () => Stream.instances.at(-1)!;
const ids = () => current.events.map((e) => e.id);

beforeEach(() => {
  vi.useFakeTimers();
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  vi.stubGlobal("EventSource", Stream);
  Stream.instances = [];
  requests = [];
  list
    .mockReset()
    .mockImplementation(() => new Promise((resolve) => requests.push(resolve)));
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
});
afterEach(async () => {
  await act(async () => root.unmount());
  container.remove();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("observe stream lifecycle", () => {
  it("batches live records, preserves latest payloads and bounds the merged list", async () => {
    await render();
    await act(async () => requests[0]([event("old"), event("a")]));
    await live(true);
    stream().emit(event("a"));
    stream().emit(event("b"));
    stream().emit({ ...event("a"), error: "updated" });
    expect(ids()).toEqual(["old", "a"]);
    await tick();
    expect(ids()).toEqual(["a", "b", "old"]);
    expect(current.events[0].error).toBe("updated");
  });

  it("discards pending records and late list responses from the previous project", async () => {
    await render();
    await live(true);
    const previous = stream();
    previous.emit(event("stale-live"));
    await render("b");
    expect(previous.readyState).toBe(Stream.CLOSED);
    await act(async () => requests[1]([event("new-project")]));
    await act(async () => requests[0]([event("old-project")]));
    await tick();
    expect(ids()).toEqual(["new-project"]);
    expect(stream().url).toMatch(/^b\?/);
  });

  it("does not show old initial results while a new project is loading", async () => {
    await render();
    await act(async () => requests[0]([event("old-project")]));
    await render("b");
    expect(current.loading).toBe(true);
    expect(ids()).toEqual([]);
  });

  it("cancels queued events on name changes and filters the live stream by name", async () => {
    await render("a", "Read");
    await live(true);
    stream().emit(event("old-read"));
    await render("a", "Write");
    stream().emit(event("wrong-name", "Read"));
    stream().emit(event("write", "Write"));
    await tick();
    expect(ids()).toEqual(["write"]);
  });

  it("cancels pending events on pause and resumes with a fresh connection", async () => {
    await render();
    await live(true);
    stream().emit(event("visible"));
    await tick();
    const previous = stream();
    previous.emit(event("pending"));
    await live(false);
    await tick();
    expect(ids()).toEqual(["visible"]);
    expect(previous.readyState).toBe(Stream.CLOSED);
    await live(true);
    expect(stream()).not.toBe(previous);
    stream().emit(event("resumed"));
    await tick();
    expect(ids()).toEqual(["resumed", "visible"]);
  });

  it("closes the stream and cancels its pending batch on unmount", async () => {
    await render();
    await live(true);
    stream().emit(event("pending"));
    expect(vi.getTimerCount()).toBe(1);
    await act(async () => root.render(null));
    expect(stream().readyState).toBe(Stream.CLOSED);
    expect(vi.getTimerCount()).toBe(0);
  });
});
