import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { MoreHorizontal } from "lucide-react";
import { toast } from "sonner";
import { api, ApiError } from "@/lib/api";
import type { Device, DevicesResponse, DriverOption, EnrollResponse, RotateResponse } from "@/lib/types";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { OnlineDot, StatusBadge } from "@/components/common/StatusViz";
import { RelTime } from "@/components/common/RelTime";
import { EmptyState } from "@/components/common/EmptyState";
import { RevealDialog } from "@/components/common/RevealDialog";
import { ConfirmDialog } from "@/components/common/ConfirmDialog";

const UNASSIGNED = "__none__";

function DeviceRow({ device, drivers }: { device: Device; drivers: DriverOption[] }) {
  const qc = useQueryClient();
  const invalidate = () => qc.invalidateQueries({ queryKey: ["devices"] });
  const [confirmRotate, setConfirmRotate] = useState(false);
  const [confirmRevoke, setConfirmRevoke] = useState(false);
  const [reveal, setReveal] = useState<{ password: string; note: string } | null>(null);

  const assign = useMutation({
    mutationFn: (driverId: string) =>
      api.put<void>(`/api/devices/${device.id}/driver`, { driverId }),
    onSuccess: invalidate,
    onError: (e) => toast.error(e instanceof ApiError ? e.message : "Assign failed."),
  });

  const rotate = useMutation({
    mutationFn: () => api.post<RotateResponse>(`/api/devices/${device.id}/rotate`),
    onSuccess: (r) => {
      setConfirmRotate(false);
      setReveal({
        password: r.password,
        note: r.delivered
          ? "Delivered to the device. Store it now — it will not be shown again."
          : "Device is offline — staged. It applies when the device reconnects. Store it now — it will not be shown again.",
      });
      invalidate();
    },
    onError: (e) => {
      setConfirmRotate(false);
      toast.error(e instanceof ApiError ? e.message : "Rotation failed.");
    },
  });

  const revoke = useMutation({
    mutationFn: () => api.post<void>(`/api/devices/${device.id}/revoke`),
    onSuccess: () => {
      setConfirmRevoke(false);
      toast.success("Credential revoked.");
      invalidate();
    },
    onError: (e) => {
      setConfirmRevoke(false);
      toast.error(e instanceof ApiError ? e.message : "Revoke failed.");
    },
  });

  return (
    <TableRow>
      <TableCell>
        <div className="flex items-center gap-2">
          <OnlineDot online={device.online} />
          <span>{device.name || <span className="font-mono">{device.id}</span>}</span>
        </div>
        {device.name ? <span className="block font-mono text-xs text-muted-foreground">{device.id}</span> : null}
      </TableCell>
      <TableCell>
        {device.online ? <StatusBadge status="online" label="live" /> : <span className="text-muted-foreground"><RelTime at={device.lastSeenAt} /></span>}
      </TableCell>
      <TableCell className="font-mono">
        {device.reportedVersion || <span className="text-muted-foreground">—</span>}
      </TableCell>
      <TableCell>
        {drivers.length ? (
          <Select
            value={device.driverId || UNASSIGNED}
            onValueChange={(v) => assign.mutate(v === UNASSIGNED ? "" : v)}
          >
            <SelectTrigger className="h-8 w-44">
              <SelectValue placeholder="Assign driver…" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={UNASSIGNED}>Unassigned</SelectItem>
              {drivers.map((d) => (
                <SelectItem key={d.id} value={d.id}>
                  {d.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        ) : (
          <span className="text-muted-foreground">No drivers yet</span>
        )}
      </TableCell>
      <TableCell className="text-right">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="icon" aria-label="Device actions">
              <MoreHorizontal className="size-4" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem onSelect={() => setConfirmRotate(true)}>
              Rotate credential
            </DropdownMenuItem>
            <DropdownMenuItem variant="destructive" onSelect={() => setConfirmRevoke(true)}>
              Revoke credential
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </TableCell>

      <ConfirmDialog
        open={confirmRotate}
        onOpenChange={setConfirmRotate}
        title="Rotate credential?"
        description="Issues a new MQTT password. The current one stays valid until the device adopts the new one, so it cannot be locked out."
        confirmLabel="Rotate"
        loading={rotate.isPending}
        onConfirm={() => rotate.mutate()}
      />
      <ConfirmDialog
        open={confirmRevoke}
        onOpenChange={setConfirmRevoke}
        title="Revoke credential?"
        description="Deletes the device credential and disconnects it. It must be re-enrolled to reconnect."
        confirmLabel="Revoke"
        destructive
        loading={revoke.isPending}
        onConfirm={() => revoke.mutate()}
      />
      <RevealDialog
        open={reveal !== null}
        onOpenChange={(o) => !o && setReveal(null)}
        title="New credential"
        description={reveal?.note}
        value={reveal?.password ?? ""}
        note="Store it now — it will not be shown again."
      />
    </TableRow>
  );
}

export function Devices() {
  const { data, isLoading } = useQuery({
    queryKey: ["devices"],
    queryFn: () => api.get<DevicesResponse>("/api/devices"),
  });
  const [enrolled, setEnrolled] = useState<EnrollResponse | null>(null);

  const enroll = useMutation({
    mutationFn: () => api.post<EnrollResponse>("/api/devices/enroll"),
    onSuccess: setEnrolled,
    onError: (e) => toast.error(e instanceof ApiError ? e.message : "Enroll failed."),
  });

  return (
    <Card>
      <div className="flex items-center gap-3 border-b border-border px-4 py-3">
        <h2 className="text-sm font-medium">Devices</h2>
        <span className="text-xs text-muted-foreground">{data?.devices.length ?? 0} total</span>
        <Button className="ml-auto" size="sm" disabled={enroll.isPending} onClick={() => enroll.mutate()}>
          Enroll device
        </Button>
      </div>

      {isLoading ? (
        <div className="space-y-2 p-4">
          <Skeleton className="h-8" />
          <Skeleton className="h-8" />
          <Skeleton className="h-8" />
        </div>
      ) : data && data.devices.length ? (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Device</TableHead>
              <TableHead>Last seen</TableHead>
              <TableHead>Firmware</TableHead>
              <TableHead>Driver</TableHead>
              <TableHead className="text-right">Actions</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {data.devices.map((d) => (
              <DeviceRow key={d.id} device={d} drivers={data.drivers} />
            ))}
          </TableBody>
        </Table>
      ) : (
        <EmptyState title="No devices yet">
          Enroll a device to mint a claim token, then flash it — it adopts its credential over MQTT
          and appears here.
        </EmptyState>
      )}

      <RevealDialog
        open={enrolled !== null}
        onOpenChange={(o) => !o && setEnrolled(null)}
        title="Device enrolled"
        description={enrolled ? `Claim token expires ${new Date(enrolled.expiresAt).toLocaleString()}.` : undefined}
        value={enrolled?.token ?? ""}
      />
    </Card>
  );
}
