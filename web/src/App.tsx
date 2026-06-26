import { Navigate, Route, Routes } from "react-router-dom";
import { ProtectedRoute } from "@/auth/ProtectedRoute";
import { AppShell } from "@/components/layout/AppShell";
import { Login } from "@/pages/Login";
import { Overview } from "@/pages/Overview";
import { Devices } from "@/pages/Devices";
import { Drivers } from "@/pages/Drivers";
import { Deploys } from "@/pages/Deploys";
import { DeployDetail } from "@/pages/DeployDetail";

export function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route element={<ProtectedRoute />}>
        <Route element={<AppShell />}>
          <Route path="/" element={<Overview />} />
          <Route path="/devices" element={<Devices />} />
          <Route path="/drivers" element={<Drivers />} />
          <Route path="/deploys" element={<Deploys />} />
          <Route path="/deploys/:id" element={<DeployDetail />} />
        </Route>
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}
