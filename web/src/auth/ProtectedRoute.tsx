import { Navigate, Outlet } from "react-router-dom";
import { useAuth } from "./AuthProvider";

export function ProtectedRoute() {
  const { loading, authenticated } = useAuth();
  if (loading) {
    return <div className="grid h-screen place-items-center text-muted-foreground">Loading…</div>;
  }
  if (!authenticated) {
    return <Navigate to="/login" replace />;
  }
  return <Outlet />;
}
