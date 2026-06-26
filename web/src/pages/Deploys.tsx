import { Link } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import type { DeployRow } from "@/lib/types";
import { Card } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { SegmentedProgress, StatusBadge } from "@/components/common/StatusViz";
import { EmptyState } from "@/components/common/EmptyState";
import { RelTime } from "@/components/common/RelTime";

function ProgressCell({ d }: { d: DeployRow }) {
  const c = d.counts;
  return (
    <div className="min-w-48">
      <SegmentedProgress counts={c} />
      <div className="mt-1 text-xs text-muted-foreground">
        {c.succeeded}/{c.total}
        {c.failed ? (
          <span style={{ color: "var(--status-fail)" }}> · {c.failed} failed</span>
        ) : null}
        {c.lost ? <span style={{ color: "var(--status-warn)" }}> · {c.lost} lost</span> : null}
        {!c.atRisk && c.succeeded === 0 ? " · awaiting first trigger" : null}
      </div>
    </div>
  );
}

export function Deploys() {
  const { data, isLoading } = useQuery({
    queryKey: ["deploys"],
    queryFn: () => api.get<DeployRow[]>("/api/deploys"),
  });

  return (
    <Card>
      <div className="flex items-center gap-3 border-b border-border px-4 py-3">
        <h2 className="text-sm font-medium">Deploys</h2>
        <span className="text-xs text-muted-foreground">{data?.length ?? 0} total</span>
      </div>
      {isLoading ? (
        <div className="space-y-2 p-4">
          <Skeleton className="h-8" />
          <Skeleton className="h-8" />
        </div>
      ) : data && data.length ? (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Driver</TableHead>
              <TableHead>Version</TableHead>
              <TableHead>State</TableHead>
              <TableHead>Progress</TableHead>
              <TableHead>Created</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {data.map((d) => (
              <TableRow key={d.id}>
                <TableCell style={d.counts.atRisk ? { boxShadow: "inset 3px 0 0 var(--status-fail)" } : undefined}>
                  {d.driver}
                </TableCell>
                <TableCell className="font-mono">
                  <Link to={`/deploys/${d.id}`} className="text-primary hover:underline">
                    {d.version}
                  </Link>
                </TableCell>
                <TableCell>
                  <StatusBadge status={d.state} label={d.stateText} />
                </TableCell>
                <TableCell>
                  <ProgressCell d={d} />
                </TableCell>
                <TableCell className="text-muted-foreground">
                  <RelTime at={d.createdAt} />
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      ) : (
        <EmptyState title="No deploys yet">
          A deploy appears here when firmware is built and rolled out to a driver's fleet.
        </EmptyState>
      )}
    </Card>
  );
}
