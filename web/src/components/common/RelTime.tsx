// Mirrors the server's reltime buckets: null -> "never", future/<60s -> "just
// now", then Nm / Nh / Nd ago (floored).
export function RelTime({ at }: { at: string | null | undefined }) {
  if (!at) return <span>never</span>;
  const d = new Date(at);
  const ms = Date.now() - d.getTime();
  let text: string;
  if (ms < 60_000) text = "just now";
  else if (ms < 3_600_000) text = `${Math.floor(ms / 60_000)}m ago`;
  else if (ms < 86_400_000) text = `${Math.floor(ms / 3_600_000)}h ago`;
  else text = `${Math.floor(ms / 86_400_000)}d ago`;
  return (
    <time dateTime={at} title={d.toLocaleString()}>
      {text}
    </time>
  );
}
