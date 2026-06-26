import { Fragment } from "react";
import type { DeployCounts } from "@/lib/types";

const GROUP = {
  ok: { fg: "var(--status-ok)", bg: "var(--status-ok-bg)" },
  active: { fg: "var(--status-active)", bg: "var(--status-active-bg)" },
  warn: { fg: "var(--status-warn)", bg: "var(--status-warn-bg)" },
  fail: { fg: "var(--status-fail)", bg: "var(--status-fail-bg)" },
  neutral: { fg: "var(--muted-foreground)", bg: "var(--status-neutral-bg)" },
} as const;

function groupFor(status: string): keyof typeof GROUP {
  switch (status) {
    case "online":
    case "succeeded":
    case "completed":
      return "ok";
    case "triggered":
    case "downloading":
    case "in_progress":
      return "active";
    case "paused":
    case "lost":
      return "warn";
    case "failed":
      return "fail";
    default:
      return "neutral";
  }
}

export function StatusBadge({ status, label }: { status: string; label?: string }) {
  const g = GROUP[groupFor(status)];
  return (
    <span
      className="inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-xs font-medium capitalize whitespace-nowrap"
      style={{ color: g.fg, backgroundColor: g.bg }}
    >
      <span className="size-1.5 rounded-full" style={{ backgroundColor: "currentColor" }} />
      {label ?? status}
    </span>
  );
}

export function OnlineDot({ online }: { online: boolean }) {
  return (
    <span
      role="img"
      aria-label={online ? "online" : "offline"}
      className="inline-block size-2 shrink-0 rounded-full"
      style={
        online
          ? { backgroundColor: "var(--status-ok)", boxShadow: "0 0 0 3px var(--status-ok-bg)" }
          : { border: "1.5px solid var(--slate-500)" }
      }
    />
  );
}

export function SegmentedProgress({ counts }: { counts: DeployCounts }) {
  const segs = [
    { pct: counts.succeededPct, color: "var(--status-ok)" },
    { pct: counts.inflightPct, color: "var(--status-active)" },
    { pct: counts.failedPct, color: "var(--status-fail)" },
    { pct: counts.lostPct, color: "var(--status-warn)" },
  ];
  return (
    <div
      className="flex h-2 w-full overflow-hidden rounded-full"
      style={{ backgroundColor: "var(--secondary)" }}
      role="img"
      aria-label={`${counts.succeeded} of ${counts.total} succeeded, ${counts.inflight} in flight, ${counts.failed} failed, ${counts.lost} lost`}
    >
      {segs.map((s, i) =>
        s.pct > 0 ? (
          <div key={i} style={{ width: `${s.pct}%`, backgroundColor: s.color }} />
        ) : null,
      )}
    </div>
  );
}

export function CountsLegend({ counts }: { counts: DeployCounts }) {
  const parts: { key: string; node: React.ReactNode }[] = [
    { key: "done", node: `${counts.succeeded}/${counts.total} done` },
  ];
  if (counts.inflight) parts.push({ key: "inflight", node: `${counts.inflight} in flight` });
  if (counts.pending) parts.push({ key: "pending", node: `${counts.pending} pending` });
  if (counts.failed)
    parts.push({
      key: "failed",
      node: <span style={{ color: "var(--status-fail)" }}>{counts.failed} failed</span>,
    });
  if (counts.lost)
    parts.push({
      key: "lost",
      node: <span style={{ color: "var(--status-warn)" }}>{counts.lost} lost</span>,
    });
  return (
    <span className="text-xs text-muted-foreground">
      {parts.map((p, i) => (
        <Fragment key={p.key}>
          {i > 0 ? " · " : ""}
          {p.node}
        </Fragment>
      ))}
    </span>
  );
}
