import { useEffect, useRef, useState } from "react";

export type EventStreamStatus = "idle" | "connecting" | "open" | "closed" | "error";

interface UseEventStreamOptions<T> {
  /** Toggles the connection on/off without changing the URL. */
  enabled: boolean;
  /** Called for each parsed message; receives the decoded JSON payload. */
  onEvent: (event: T) => void;
  /** Optional handler for non-JSON errors (parsing failures are swallowed). */
  onError?: (err: Event) => void;
}

interface UseEventStreamResult {
  status: EventStreamStatus;
}

export function useEventStream<T>(
  url: string,
  { enabled, onEvent, onError }: UseEventStreamOptions<T>,
): UseEventStreamResult {
  const [status, setStatus] = useState<EventStreamStatus>("idle");
  const onEventRef = useRef(onEvent);
  const onErrorRef = useRef(onError);

  // Latest-callback ref so parent re-renders don't tear down the stream.
  useEffect(() => {
    onEventRef.current = onEvent;
    onErrorRef.current = onError;
  }, [onEvent, onError]);

  useEffect(() => {
    if (!enabled || !url) {
      setStatus("idle");
      return;
    }

    setStatus("connecting");
    const source = new EventSource(url);

    source.onopen = () => setStatus("open");

    source.onmessage = (e) => {
      try {
        const parsed = JSON.parse(e.data) as T;
        onEventRef.current(parsed);
      } catch {
        // Malformed event — skip without dropping the connection.
      }
    };

    source.onerror = (err) => {
      setStatus(source.readyState === EventSource.CLOSED ? "closed" : "error");
      onErrorRef.current?.(err);
    };

    return () => {
      source.close();
      setStatus("closed");
    };
  }, [url, enabled]);

  return { status };
}
