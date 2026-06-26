import { ESPLoader, Transport } from "esptool-js";

export function webSerialSupported(): boolean {
  return typeof navigator !== "undefined" && "serial" in navigator && window.isSecureContext;
}

export async function requestPort(): Promise<SerialPort> {
  return navigator.serial.requestPort();
}

interface ManifestPart {
  path: string;
  offset: number;
}
interface Manifest {
  builds: { chipFamily: string; parts: ManifestPart[] }[];
}

// flashAgent flashes the embedded base firmware onto the connected ESP using
// esptool-js, then resets it so it boots into the serial provisioning agent.
export async function flashAgent(
  port: SerialPort,
  manifestUrl: string,
  onProgress: (pct: number) => void,
): Promise<void> {
  const transport = new Transport(port, true);
  try {
    const loader = new ESPLoader({ transport, baudrate: 115200 });
    await loader.main();

    const manifest: Manifest = await (await fetch(manifestUrl)).json();
    const build = manifest.builds[0];
    const fileArray: { data: Uint8Array; address: number }[] = [];
    for (const part of build.parts) {
      const buf = await (await fetch(part.path)).arrayBuffer();
      fileArray.push({ data: new Uint8Array(buf), address: part.offset });
    }
    // esptool-js reports progress in compressed bytes, so the denominator must
    // be the compressed per-file total (the 3rd callback arg), not the raw size.
    const written = new Array(fileArray.length).fill(0);
    const totals = new Array(fileArray.length).fill(0);
    await loader.writeFlash({
      fileArray,
      flashSize: "keep",
      flashMode: "keep",
      flashFreq: "keep",
      eraseAll: true,
      compress: true,
      reportProgress: (fileIndex: number, sent: number, fileTotal: number) => {
        written[fileIndex] = sent;
        totals[fileIndex] = fileTotal;
        const den = totals.reduce((a, b) => a + b, 0);
        if (den > 0) {
          onProgress(Math.min(99, Math.round((written.reduce((a, b) => a + b, 0) / den) * 100)));
        }
      },
    });
    await loader.after();
  } finally {
    // Always release the port so a retry (or the serial handoff) can reopen it.
    await transport.disconnect().catch(() => {});
  }
}

// EspmSerial speaks the device's ASCII provisioning protocol over the same port
// after flashing: ESPM:GETMAC -> ESPM:READY <mac>, ESPM:SET <k> <v> -> ESPM:OK,
// ESPM:PROVISION -> ESPM:OK PROVISIONED (then the device reboots).
export class EspmSerial {
  private reader: ReadableStreamDefaultReader<Uint8Array> | null = null;
  private writer: WritableStreamDefaultWriter<Uint8Array> | null = null;
  private pending: Promise<ReadableStreamReadResult<Uint8Array>> | null = null;
  private closed = false;
  private buf = "";
  private decoder = new TextDecoder();
  private encoder = new TextEncoder();

  constructor(private port: SerialPort) {}

  async open(): Promise<void> {
    if (!this.port.readable) await this.port.open({ baudRate: 115200 });
    this.reader = this.port.readable!.getReader();
    try {
      this.writer = this.port.writable!.getWriter();
    } catch (e) {
      this.reader.releaseLock();
      this.reader = null;
      throw e;
    }
  }

  async close(): Promise<void> {
    try {
      await this.reader?.cancel();
      this.reader?.releaseLock();
      this.writer?.releaseLock();
    } catch {
      // ignore
    }
    try {
      await this.port.close();
    } catch {
      // ignore
    }
  }

  private async readLine(timeoutMs = 4000): Promise<string | null> {
    if (this.closed) throw new Error("Serial stream closed (device disconnected).");
    const deadline = Date.now() + timeoutMs;
    for (;;) {
      const nl = this.buf.indexOf("\n");
      if (nl >= 0) {
        const line = this.buf.slice(0, nl).replace(/\r$/, "");
        this.buf = this.buf.slice(nl + 1);
        return line;
      }
      const remaining = deadline - Date.now();
      if (remaining <= 0) return null;
      // Keep the in-flight read across timeouts: a reader rejects a second
      // read() while one is pending, so we must not start a new one each loop.
      if (!this.pending) this.pending = this.reader!.read();
      const timedOut = Symbol("timeout");
      const result = await Promise.race([
        this.pending,
        new Promise<typeof timedOut>((res) => setTimeout(() => res(timedOut), remaining)),
      ]);
      if (result === timedOut) return null;
      this.pending = null;
      // A closed stream resolves read() with done forever; fail fast instead of
      // busy-spinning new reads against it.
      if (result.done) {
        this.closed = true;
        throw new Error("Serial stream closed (device disconnected).");
      }
      if (result.value) this.buf += this.decoder.decode(result.value);
    }
  }

  private async send(line: string): Promise<void> {
    await this.writer!.write(this.encoder.encode(line + "\n"));
  }

  // getMac probes the agent until it answers ESPM:READY <mac>.
  async getMac(): Promise<string> {
    for (let attempt = 0; attempt < 8; attempt++) {
      await this.send("ESPM:GETMAC");
      const start = Date.now();
      while (Date.now() - start < 2000) {
        const line = await this.readLine(2000);
        if (line && line.startsWith("ESPM:READY")) {
          const parts = line.split(" ");
          if (parts.length >= 2) return parts[1];
        }
      }
    }
    throw new Error("Device did not enter provisioning mode (no ESPM:READY).");
  }

  private async setKey(key: string, value: string): Promise<void> {
    await this.send(`ESPM:SET ${key} ${value}`);
    const r = await this.readLine();
    if (r !== "ESPM:OK") throw new Error(`Device rejected ${key} (${r ?? "no reply"}).`);
  }

  async provision(cfg: {
    ssid: string;
    pass: string;
    host: string;
    hport: number;
    mport: number;
    token: string;
  }): Promise<void> {
    await this.setKey("ssid", cfg.ssid);
    await this.setKey("pass", cfg.pass);
    await this.setKey("host", cfg.host);
    await this.setKey("hport", String(cfg.hport));
    await this.setKey("mport", String(cfg.mport));
    await this.setKey("token", cfg.token);
    await this.send("ESPM:PROVISION");
    const r = await this.readLine();
    if (!r || !r.startsWith("ESPM:OK")) throw new Error(`Provisioning failed (${r ?? "no reply"}).`);
  }
}
