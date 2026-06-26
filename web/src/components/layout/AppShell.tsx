import { NavLink, Outlet, useLocation } from "react-router-dom";
import { Cpu, GitBranch, LayoutGrid, LogOut, Rocket } from "lucide-react";
import { useAuth } from "@/auth/AuthProvider";
import { useEvents } from "@/hooks/useEvents";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

const NAV = [
  { to: "/", label: "Overview", icon: LayoutGrid, end: true },
  { to: "/devices", label: "Devices", icon: Cpu, end: false },
  { to: "/drivers", label: "Drivers", icon: GitBranch, end: false },
  { to: "/deploys", label: "Deploys", icon: Rocket, end: false },
];

function titleFor(pathname: string): string {
  if (pathname.startsWith("/devices")) return "Devices";
  if (pathname.startsWith("/drivers")) return "Drivers";
  if (pathname.startsWith("/deploys")) return "Deploys";
  return "Overview";
}

export function AppShell() {
  const { user, logout } = useAuth();
  const live = useEvents();
  const { pathname } = useLocation();

  return (
    <div className="grid h-dvh grid-cols-[var(--sidebar-w)_1fr] grid-rows-[var(--header-h)_1fr]">
      <aside className="row-span-2 flex flex-col border-r border-border bg-sidebar">
        <div className="flex h-(--header-h) items-center gap-2 border-b border-border px-4 font-medium">
          <span className="grid size-6 place-items-center rounded-md bg-primary/15 text-xs text-primary">
            E
          </span>
          ESPManager
        </div>
        <nav className="flex flex-1 flex-col gap-0.5 p-2">
          {NAV.map(({ to, label, icon: Icon, end }) => (
            <NavLink
              key={to}
              to={to}
              end={end}
              className={({ isActive }) =>
                cn(
                  "flex items-center gap-3 rounded-md px-3 py-2 text-sm text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground",
                  isActive && "bg-primary/15 text-foreground",
                )
              }
            >
              <Icon className="size-4" />
              {label}
            </NavLink>
          ))}
        </nav>
        <div className="flex items-center gap-2 border-t border-border px-4 py-3 text-xs text-muted-foreground">
          <span
            className="size-2 rounded-full"
            style={
              live === "open"
                ? { backgroundColor: "var(--status-ok)", boxShadow: "0 0 0 3px var(--status-ok-bg)" }
                : { backgroundColor: "var(--status-warn)", boxShadow: "0 0 0 3px var(--status-warn-bg)" }
            }
          />
          {live === "open" ? "Live" : "Reconnecting"}
        </div>
      </aside>

      <header className="col-start-2 flex items-center gap-3 border-b border-border px-5">
        <h1 className="text-base font-medium">{titleFor(pathname)}</h1>
        <div className="ml-auto flex items-center gap-3 text-sm text-muted-foreground">
          <span>{user}</span>
          <Button variant="ghost" size="sm" onClick={() => void logout()}>
            <LogOut className="size-4" />
            Log out
          </Button>
        </div>
      </header>

      <main className="col-start-2 overflow-y-auto p-5">
        <Outlet />
      </main>
    </div>
  );
}
