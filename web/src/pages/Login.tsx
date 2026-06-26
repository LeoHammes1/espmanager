import { useState } from "react";
import { Navigate, useNavigate } from "react-router-dom";
import { Eye, EyeOff } from "lucide-react";
import { ApiError } from "@/lib/api";
import { useAuth } from "@/auth/AuthProvider";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Alert, AlertDescription } from "@/components/ui/alert";

export function Login() {
  const { login, authenticated, setupRequired, loading } = useAuth();
  const navigate = useNavigate();
  const [password, setPassword] = useState("");
  const [show, setShow] = useState(false);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  if (authenticated) return <Navigate to="/" replace />;

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      await login(password);
      navigate("/", { replace: true });
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Sign in failed.");
      setPassword("");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="grid min-h-dvh place-items-center p-6">
      <div className="flex w-full max-w-sm flex-col gap-5">
        <div className="flex items-center justify-center gap-2 text-base font-medium">
          <span className="grid size-7 place-items-center rounded-md bg-primary/15 text-primary">E</span>
          ESPManager
        </div>
        <div className="rounded-lg border border-border bg-card p-5">
          {loading ? (
            <p className="text-sm text-muted-foreground">Loading…</p>
          ) : setupRequired ? (
            <Alert>
              <AlertDescription>
                Set <code className="font-mono">ESPM_ADMIN_PASSWORD</code> and restart the server to
                enable sign-in.
              </AlertDescription>
            </Alert>
          ) : (
            <form onSubmit={onSubmit} className="flex flex-col gap-4">
              {error ? (
                <Alert variant="destructive">
                  <AlertDescription>{error}</AlertDescription>
                </Alert>
              ) : null}
              <div className="flex flex-col gap-2">
                <Label htmlFor="password">Password</Label>
                <div className="flex gap-2">
                  <Input
                    id="password"
                    type={show ? "text" : "password"}
                    autoComplete="current-password"
                    autoFocus
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                  />
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    aria-label={show ? "Hide password" : "Show password"}
                    onClick={() => setShow((s) => !s)}
                  >
                    {show ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
                  </Button>
                </div>
              </div>
              <Button type="submit" disabled={busy}>
                {busy ? "Signing in…" : "Sign in"}
              </Button>
            </form>
          )}
        </div>
      </div>
    </div>
  );
}
