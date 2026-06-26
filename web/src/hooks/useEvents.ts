import { useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import type { LiveStatus } from "@/lib/types";

// useEvents subscribes to the server's SSE tick and refetches all live data on
// each change (debounced). The server emits a generic "changed" signal, so the
// hook simply invalidates the React Query cache. Returns the connection status
// for the sidebar live indicator.
export function useEvents(): LiveStatus {
  const qc = useQueryClient();
  const [status, setStatus] = useState<LiveStatus>("connecting");

  useEffect(() => {
    const es = new EventSource("/events", { withCredentials: true });
    let timer: number | undefined;
    es.onopen = () => setStatus("open");
    es.onerror = () => setStatus("connecting");
    es.onmessage = () => {
      window.clearTimeout(timer);
      timer = window.setTimeout(() => qc.invalidateQueries(), 300);
    };
    return () => {
      window.clearTimeout(timer);
      es.close();
    };
  }, [qc]);

  return status;
}
