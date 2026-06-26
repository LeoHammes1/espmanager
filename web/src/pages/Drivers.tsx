import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, ApiError } from "@/lib/api";
import type { CreateDriverResponse, Driver, ListDriversResponse } from "@/lib/types";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { CopyButton } from "@/components/common/CopyButton";
import { EmptyState } from "@/components/common/EmptyState";
import { RelTime } from "@/components/common/RelTime";
import { RevealDialog } from "@/components/common/RevealDialog";

const EMPTY = { name: "", repoUrl: "", branch: "", pioEnv: "" };

export function Drivers() {
  const qc = useQueryClient();
  const { data, isLoading } = useQuery({
    queryKey: ["drivers"],
    queryFn: () => api.get<ListDriversResponse>("/api/drivers"),
  });
  const [form, setForm] = useState(EMPTY);
  const [created, setCreated] = useState<CreateDriverResponse | null>(null);

  const create = useMutation({
    mutationFn: () => api.post<CreateDriverResponse>("/api/drivers", form),
    onSuccess: (r) => {
      setCreated(r);
      setForm(EMPTY);
      void qc.invalidateQueries({ queryKey: ["drivers"] });
    },
    onError: (e) => toast.error(e instanceof ApiError ? e.message : "Could not create driver."),
  });

  const set = (k: keyof typeof EMPTY) => (e: React.ChangeEvent<HTMLInputElement>) =>
    setForm((f) => ({ ...f, [k]: e.target.value }));

  return (
    <div className="flex flex-col gap-5">
      <Card>
        <div className="flex items-center gap-3 border-b border-border px-4 py-3">
          <h2 className="text-sm font-medium">Drivers</h2>
          <span className="text-xs text-muted-foreground">{data?.drivers.length ?? 0} total</span>
        </div>
        {isLoading ? (
          <div className="space-y-2 p-4">
            <Skeleton className="h-8" />
            <Skeleton className="h-8" />
          </div>
        ) : data && data.drivers.length ? (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Driver</TableHead>
                <TableHead>Source</TableHead>
                <TableHead>Env</TableHead>
                <TableHead>Webhook</TableHead>
                <TableHead>Created</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {data.drivers.map((d: Driver) => (
                <TableRow key={d.id}>
                  <TableCell>
                    {d.name}
                    <span className="block font-mono text-xs text-muted-foreground">{d.id}</span>
                  </TableCell>
                  <TableCell>
                    {d.repoUrl}
                    <span className="block text-xs text-muted-foreground">{d.branch}</span>
                  </TableCell>
                  <TableCell className="font-mono">
                    {d.pioEnv || <span className="text-muted-foreground">—</span>}
                  </TableCell>
                  <TableCell>
                    <CopyButton value={d.webhookUrl} label="Copy URL" />
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    <RelTime at={d.createdAt} />
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        ) : (
          <EmptyState title="No drivers yet">
            A driver links a firmware repository to the devices that run it. Create one below.
          </EmptyState>
        )}
      </Card>

      <Card className="p-4">
        <h2 className="mb-1 text-sm font-medium">New driver</h2>
        <p className="mb-4 text-sm text-muted-foreground">
          Maps a git repository and branch to a PlatformIO build. Pushes build and roll out
          automatically.
        </p>
        <form
          className="flex max-w-lg flex-col gap-4"
          onSubmit={(e) => {
            e.preventDefault();
            create.mutate();
          }}
        >
          <div className="flex flex-col gap-2">
            <Label htmlFor="name">Name</Label>
            <Input id="name" value={form.name} onChange={set("name")} required />
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="repoUrl">Repository URL</Label>
            <Input
              id="repoUrl"
              value={form.repoUrl}
              onChange={set("repoUrl")}
              placeholder="https://github.com/you/firmware.git"
              required
            />
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="branch">Branch</Label>
            <Input id="branch" value={form.branch} onChange={set("branch")} placeholder="main" />
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="pioEnv">PlatformIO env</Label>
            <Input id="pioEnv" value={form.pioEnv} onChange={set("pioEnv")} placeholder="esp32dev" />
          </div>
          <Button type="submit" className="self-start" disabled={create.isPending}>
            {create.isPending ? "Creating…" : "Create driver"}
          </Button>
        </form>
      </Card>

      <RevealDialog
        open={created !== null}
        onOpenChange={(o) => !o && setCreated(null)}
        title={created ? `Driver “${created.driver.name}” created` : "Driver created"}
        description="Add this webhook secret to your git host. It signs the push payloads."
        value={created?.webhookSecret ?? ""}
      />
    </div>
  );
}
