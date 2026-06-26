import { Link, useParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { AlertTriangle } from "lucide-react";
import { api, ApiError } from "@/lib/api";
import type { BatchView, DeployDetail as DeployDetailType } from "@/lib/types";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Alert, AlertDescription } from "@/components/ui/alert";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { CountsLegend, SegmentedProgress, StatusBadge } from "@/components/common/StatusViz";
import { EmptyState } from "@/components/common/EmptyState";
import { RelTime } from "@/components/common/RelTime";
import { useDeployActions } from "@/hooks/useDeployActions";

function BatchPanel({ batch }: { batch: BatchView }) {
  return (
    <Card>
      <div className="flex items-center gap-3 border-b border-border px-4 py-3">
        <span
          className="rounded px-2 py-0.5 text-xs"
          style={
            batch.batch === 0
              ? { color: "var(--status-active)", backgroundColor: "var(--status-active-bg)" }
              : { color: "var(--muted-foreground)", backgroundColor: "var(--secondary)" }
          }
        >
          {batch.label}
        </span>
        <span className="ml-auto">
          <CountsLegend counts={batch.counts} />
        </span>
      </div>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Device</TableHead>
            <TableHead>Sequence</TableHead>
            <TableHead>Status</TableHead>
            <TableHead>Updated</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {batch.targets.map((t) => (
            <TableRow key={t.deviceId}>
              <TableCell>{t.deviceName || <span className="font-mono">{t.deviceId}</span>}</TableCell>
              <TableCell className="font-mono">{t.sequence}</TableCell>
              <TableCell>
                <StatusBadge status={t.status} />
              </TableCell>
              <TableCell className="text-muted-foreground">
                <RelTime at={t.updatedAt} />
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </Card>
  );
}

export function DeployDetail() {
  const { id = "" } = useParams();
  const { resume, cancel } = useDeployActions();
  const { data, isLoading, error } = useQuery({
    queryKey: ["deploys", id],
    queryFn: () => api.get<DeployDetailType>(`/api/deploys/${id}`),
    retry: false,
  });

  if (isLoading) {
    return (
      <div className="flex flex-col gap-4">
        <Skeleton className="h-24" />
        <Skeleton className="h-40" />
      </div>
    );
  }
  if (error instanceof ApiError && error.status === 404) {
    return <EmptyState title="Deploy not found">This deploy no longer exists.</EmptyState>;
  }
  if (!data) {
    return <EmptyState title="Could not load deploy">Try again in a moment.</EmptyState>;
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="text-sm text-muted-foreground">
        <Link to="/deploys" className="text-primary hover:underline">
          Deploys
        </Link>{" "}
        / <span className="font-mono">{data.version}</span>
      </div>

      <Card>
        <div className="flex items-center gap-3 border-b border-border px-4 py-3">
          <h2 className="text-sm font-medium">{data.driver}</h2>
          <span className="font-mono text-xs text-muted-foreground">{data.version}</span>
          <div className="ml-auto flex items-center gap-3">
            <StatusBadge status={data.state} label={data.stateText} />
            <span className="text-xs text-muted-foreground">
              started <RelTime at={data.createdAt} />
            </span>
          </div>
        </div>
        <div className="flex flex-col gap-3 p-4">
          {data.pause ? (
            <Alert variant="default" style={{ borderLeft: "3px solid var(--status-warn)" }}>
              <AlertTriangle className="size-4" />
              <AlertDescription className="flex w-full flex-wrap items-center gap-3">
                <span className="flex-1">
                  Auto-paused at the <strong>{data.pause.batchLabel}</strong> batch — {data.pause.failed} failed
                  + {data.pause.lost} lost of {data.pause.total}, over the {data.pause.threshold}% threshold.
                  The rest of the fleet was not started.
                </span>
                <span className="flex gap-2">
                  <Button size="sm" variant="secondary" onClick={() => resume.mutate(data.id)}>
                    Resume
                  </Button>
                  <Button
                    size="sm"
                    variant="ghost"
                    className="text-destructive hover:text-destructive"
                    onClick={() => cancel.mutate(data.id)}
                  >
                    Cancel
                  </Button>
                </span>
              </AlertDescription>
            </Alert>
          ) : null}
          <SegmentedProgress counts={data.counts} />
          <CountsLegend counts={data.counts} />
        </div>
      </Card>

      {data.batches.map((b) => (
        <BatchPanel key={b.batch} batch={b} />
      ))}
    </div>
  );
}
