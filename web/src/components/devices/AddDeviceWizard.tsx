import { useEffect, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, Usb } from "lucide-react";
import { api, ApiError } from "@/lib/api";
import type { EnrollResponse, ProvisionConfig } from "@/lib/types";
import { EspmSerial, flashAgent, requestPort, webSerialSupported } from "@/lib/serial";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { RevealDialog } from "@/components/common/RevealDialog";

type Step = "intro" | "flashing" | "configure" | "provisioning" | "done";

export function AddDeviceWizard({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const qc = useQueryClient();
  const supported = webSerialSupported();
  const [step, setStep] = useState<Step>("intro");
  const [error, setError] = useState("");
  const [pct, setPct] = useState(0);
  const [mac, setMac] = useState("");
  const [ssid, setSsid] = useState("");
  const [wpass, setWpass] = useState("");
  const [host, setHost] = useState(window.location.hostname);
  const [hport, setHport] = useState(80);
  const [mport, setMport] = useState(1883);
  const espm = useRef<EspmSerial | null>(null);
  const activePort = useRef<SerialPort | null>(null);
  const [manualToken, setManualToken] = useState<EnrollResponse | null>(null);

  useEffect(() => {
    if (!open) return;
    api
      .get<ProvisionConfig>("/api/config")
      .then((c) => {
        if (c.host) setHost(c.host);
        if (c.httpPort) setHport(c.httpPort);
        if (c.mqttPort) setMport(c.mqttPort);
      })
      .catch(() => {});
  }, [open]);

  function reset() {
    setStep("intro");
    setError("");
    setPct(0);
    setMac("");
    setSsid("");
    setWpass("");
    espm.current = null;
    activePort.current = null;
  }

  async function releasePort() {
    await espm.current?.close().catch(() => {});
    await activePort.current?.close().catch(() => {});
    espm.current = null;
    activePort.current = null;
  }

  function close(o: boolean) {
    if (!o) {
      void releasePort();
      reset();
    }
    onOpenChange(o);
  }

  async function connectAndFlash() {
    setError("");
    let port: SerialPort;
    try {
      port = await requestPort();
    } catch {
      return; // user dismissed the port picker
    }
    activePort.current = port;
    setStep("flashing");
    setPct(0);
    try {
      await flashAgent(port, "/firmware/agent/manifest.json", setPct);
      const serial = new EspmSerial(port);
      espm.current = serial;
      await serial.open();
      const m = await serial.getMac();
      setMac(m);
      setStep("configure");
    } catch (e) {
      await releasePort();
      setError(e instanceof Error ? e.message : "Flashing failed.");
      setStep("intro");
    }
  }

  async function provision(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setStep("provisioning");
    try {
      const { token } = await api.post<EnrollResponse>("/api/devices/enroll", { mac });
      await espm.current!.provision({ ssid, pass: wpass, host, hport, mport, token });
      await espm.current!.close();
      espm.current = null;
      setStep("done");
      void qc.invalidateQueries({ queryKey: ["devices"] });
    } catch (err) {
      setError(err instanceof ApiError ? err.message : err instanceof Error ? err.message : "Provisioning failed.");
      setStep("configure");
    }
  }

  async function enrollManually() {
    try {
      setManualToken(await api.post<EnrollResponse>("/api/devices/enroll"));
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not mint a claim token.");
    }
  }

  return (
    <>
      <Dialog open={open} onOpenChange={close}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Add device</DialogTitle>
            <DialogDescription>
              Connect an ESP32 over USB — the browser flashes it and sets up WiFi and credentials.
            </DialogDescription>
          </DialogHeader>

          {error ? (
            <Alert variant="destructive">
              <AlertTriangle className="size-4" />
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          ) : null}

          {step === "intro" && (
            <div className="flex flex-col gap-4">
              {!supported && (
                <Alert>
                  <AlertDescription>
                    USB onboarding needs Chrome or Edge over a trusted HTTPS origin. Use the manual
                    flow below, or open this page in a supported browser.
                  </AlertDescription>
                </Alert>
              )}
              <Button onClick={connectAndFlash} disabled={!supported}>
                <Usb className="size-4" />
                Connect & flash over USB
              </Button>
              <button
                type="button"
                onClick={enrollManually}
                className="text-sm text-muted-foreground underline-offset-2 hover:underline"
              >
                Or enroll manually (no USB) — get a claim token
              </button>
            </div>
          )}

          {step === "flashing" && (
            <div className="flex flex-col gap-2 py-2">
              <p className="text-sm text-muted-foreground">
                Flashing the base firmware… keep the device connected.
              </p>
              <div className="h-2 overflow-hidden rounded-full bg-secondary">
                <div
                  className="h-full"
                  style={{ width: `${pct}%`, backgroundColor: "var(--primary)" }}
                />
              </div>
              <span className="text-xs text-muted-foreground">{pct}%</span>
            </div>
          )}

          {step === "configure" && (
            <form onSubmit={provision} className="flex flex-col gap-4">
              <p className="text-sm text-muted-foreground">
                Device <span className="font-mono">{mac}</span> is ready. Enter its WiFi.
              </p>
              <div className="flex flex-col gap-2">
                <Label htmlFor="ssid">WiFi network</Label>
                <Input id="ssid" value={ssid} onChange={(e) => setSsid(e.target.value)} required />
              </div>
              <div className="flex flex-col gap-2">
                <Label htmlFor="wpass">WiFi password</Label>
                <Input
                  id="wpass"
                  type="password"
                  value={wpass}
                  onChange={(e) => setWpass(e.target.value)}
                />
              </div>
              <div className="flex gap-2">
                <div className="flex flex-1 flex-col gap-2">
                  <Label htmlFor="host">Manager host (reachable from the device)</Label>
                  <Input id="host" value={host} onChange={(e) => setHost(e.target.value)} required />
                </div>
                <div className="flex w-24 flex-col gap-2">
                  <Label htmlFor="hport">Port</Label>
                  <Input
                    id="hport"
                    type="number"
                    min={1}
                    max={65535}
                    value={hport}
                    onChange={(e) => setHport(Number(e.target.value))}
                    required
                  />
                </div>
              </div>
              <Button type="submit">Provision device</Button>
            </form>
          )}

          {step === "provisioning" && (
            <p className="py-4 text-sm text-muted-foreground">
              Writing WiFi and credentials over USB…
            </p>
          )}

          {step === "done" && (
            <div className="flex flex-col gap-3 py-2">
              <p className="text-sm">
                Done. <span className="font-mono">{mac}</span> is rebooting and will appear in the
                list once it connects.
              </p>
              <DialogFooter>
                <Button onClick={() => close(false)}>Close</Button>
              </DialogFooter>
            </div>
          )}
        </DialogContent>
      </Dialog>

      <RevealDialog
        open={manualToken !== null}
        onOpenChange={(o) => {
          if (!o) setManualToken(null);
        }}
        title="Claim token"
        description={
          manualToken ? `Flash it into the device. Expires ${new Date(manualToken.expiresAt).toLocaleString()}.` : undefined
        }
        value={manualToken?.token ?? ""}
      />
    </>
  );
}
