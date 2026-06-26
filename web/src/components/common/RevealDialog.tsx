import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { CopyButton } from "./CopyButton";

export function RevealDialog({
  open,
  onOpenChange,
  title,
  description,
  value,
  note = "Store it now — it will not be shown again.",
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: string;
  value: string;
  note?: string;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          {description ? <DialogDescription>{description}</DialogDescription> : null}
        </DialogHeader>
        <div className="rounded-md border bg-secondary p-3">
          <div className="flex items-center gap-2">
            <code className="flex-1 select-all break-all font-mono text-sm">{value}</code>
            <CopyButton value={value} />
          </div>
        </div>
        <p className="text-xs" style={{ color: "var(--status-warn)" }}>
          {note}
        </p>
        <DialogFooter>
          <Button onClick={() => onOpenChange(false)}>Done</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
