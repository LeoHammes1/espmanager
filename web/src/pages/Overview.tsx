import { Link } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { CheckCircle2 } from "lucide-react";
import { api } from "@/lib/api";
import type { DeployRow, OverviewResponse } from "@/lib/types";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Separator } from "@/components/ui/separator";
import {
  CountsLegend,
  OnlineDot,
  SegmentedProgress,
  StatusBadge,
} from "@/components/common/StatusViz";
import { EmptyState } from "@/components/common/EmptyState";
import { RelTime } from "@/components/common/RelTime";
import { useDeployActions } from "@/hooks/useDeployActions";

function RolloutCard({ d }: { d: DeployRow }) {
  const { resume, cancel } = useDeployActions();
  const paused = d.state === "paused";
  return (
    <div
      className="rounded-md border border-border p-3"
      style={d.counts.atRisk ? { borderLeft: "3px solid var(--status-fail)" } : undefined}
    >
      <div className="flex items-center gap-3">
        <Link to={`/deploys/${d.id}`} className="font-medium hover:underline">
          {d.driver}
        </Link>
        <span className="font-mono text-xs text-muted-foreground">{d.version}</span>
        <StatusBadge status={d.state} label={d.stateText} />
        {paused ? (
          <div className="ml-auto flex gap-2">
            <Button size="sm" variant="secondary" onClick={() => resume.mutate(d.id)}>
              Resume
            </Button>
            <Button
              size="sm"
              variant="ghost"
              className="text-destructive hover:text-destructive"
              onClick={() => cancel.mutate(d.id)}
            >
              Cancel
            </Button>
          </div>
        ) : null}
      </div>
      <div className="mt-2">
        <SegmentedProgress counts={d.counts} />
      </div>
      <div className="mt-2 flex items-center gap-2">
        <CountsLegend counts={d.counts} />
        <Link to={`/deploys/${d.id}`} className="text-xs text-primary hover:underline">
          View rollout
        </Link>
      </div>
    </div>
  );
}

export function Overview() {
  const { data, isLoading } = useQuery({
    queryKey: ["overview"],
    queryFn: () => api.get<OverviewResponse>("/api/overview"),
  });

  if (isLoading || !data) {
    return (
      <div className="flex flex-col gap-5">
        <div className="grid grid-cols-2 gap-4">
          <Skeleton className="h-24" />
          <Skeleton className="h-24" />
        </div>
        <Skeleton className="h-40" />
      </div>
    );
  }

  const allGood = data.offline.length === 0 && data.failedUpdates.length === 0;

  return (
    <div className="flex flex-col gap-5">
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <Link to="/devices">
          <Card className="p-4 transition-colors hover:border-border/80">
            <div className="text-xs uppercase tracking-wide text-muted-foreground">Devices online</div>
            <div className="mt-1 text-2xl">
              {data.devicesOnline} <span className="text-base text-muted-foreground">/ {data.devicesTotal}</span>
            </div>
          </Card>
        </Link>
        <Card
          className="p-4"
          style={data.attentionCount ? { borderColor: "color-mix(in srgb, var(--status-warn) 40%, var(--border))" } : undefined}
        >
          <div className="text-xs uppercase tracking-wide text-muted-foreground">Needs attention</div>
          <div className="mt-1 text-2xl" style={data.attentionCount ? { color: "var(--status-warn)" } : undefined}>
            {data.attentionCount}
          </div>
        </Card>
      </div>

      <Card>
        <div className="flex items-center gap-2 border-b border-border px-4 py-3">
          <h2 className="text-sm font-medium">Active rollouts</h2>
          {data.rollouts.length ? (
            <span className="text-xs text-muted-foreground">{data.rollouts.length}</span>
          ) : null}
        </div>
        <div className="p-4">
          {data.rollouts.length ? (
            <div className="flex flex-col gap-4">
              {data.rollouts.map((d) => (
                <RolloutCard key={d.id} d={d} />
              ))}
            </div>
          ) : (
            <EmptyState icon={<CheckCircle2 className="size-6" />} title="No active rollouts">
              Push to a driver's repo to start one — builds roll out automatically.
            </EmptyState>
          )}
        </div>
      </Card>

      <Card id="attention">
        <div className="border-b border-border px-4 py-3">
          <h2 className="text-sm font-medium">Needs attention</h2>
        </div>
        <div className="p-4">
          {allGood ? (
            <EmptyState icon={<CheckCircle2 className="size-6" />} title="All good">
              Every device is online and no update has failed.
            </EmptyState>
          ) : (
            <div className="flex flex-col gap-4">
              {data.offline.length ? (
                <div>
                  <h3 className="mb-2 text-xs uppercase tracking-wide text-muted-foreground">
                    Offline ({data.offline.length})
                  </h3>
                  <ul className="flex flex-col gap-2">
                    {data.offline.map((d) => (
                      <li key={d.id} className="flex items-center gap-2">
                        <OnlineDot online={false} />
                        {d.name || <span className="font-mono">{d.id}</span>}
                        <span className="text-xs text-muted-foreground">
                          last seen <RelTime at={d.lastSeenAt} />
                        </span>
                      </li>
                    ))}
                  </ul>
                </div>
              ) : null}
              {data.offline.length && data.failedUpdates.length ? <Separator /> : null}
              {data.failedUpdates.length ? (
                <div>
                  <h3 className="mb-2 text-xs uppercase tracking-wide text-muted-foreground">
                    Failed updates ({data.failedUpdates.length})
                  </h3>
                  <ul className="flex flex-col gap-2">
                    {data.failedUpdates.map((f, i) => (
                      <li key={`${f.deployId}-${f.deviceId}-${i}`} className="flex items-center gap-2">
                        <StatusBadge status={f.status} />
                        {f.deviceName || <span className="font-mono">{f.deviceId}</span>}
                        <span className="text-xs text-muted-foreground">
                          {f.driver} · {f.version}
                        </span>
                        <Link to={`/deploys/${f.deployId}`} className="text-xs text-primary hover:underline">
                          View
                        </Link>
                      </li>
                    ))}
                  </ul>
                </div>
              ) : null}
            </div>
          )}
        </div>
      </Card>
    </div>
  );
}
