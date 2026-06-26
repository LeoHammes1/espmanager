import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useState,
  type ReactNode,
} from "react";
import { api, onSessionExpired } from "@/lib/api";
import type { SessionState } from "@/lib/types";

type AuthContextValue = SessionState & {
  loading: boolean;
  login: (password: string) => Promise<void>;
  logout: () => Promise<void>;
  refetch: () => Promise<void>;
};

const AuthContext = createContext<AuthContextValue | null>(null);

const EMPTY: SessionState = { authenticated: false, setupRequired: false, user: "" };

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used within AuthProvider");
  return ctx;
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<SessionState>(EMPTY);
  const [loading, setLoading] = useState(true);

  const refetch = useCallback(async () => {
    try {
      setState(await api.get<SessionState>("/api/session"));
    } catch {
      setState(EMPTY);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refetch();
  }, [refetch]);

  useEffect(() => onSessionExpired(() => setState(EMPTY)), []);

  const login = useCallback(async (password: string) => {
    setState(await api.post<SessionState>("/api/session", { password }));
  }, []);

  const logout = useCallback(async () => {
    try {
      await api.del("/api/session");
    } finally {
      setState(EMPTY);
    }
  }, []);

  return (
    <AuthContext.Provider value={{ ...state, loading, login, logout, refetch }}>
      {children}
    </AuthContext.Provider>
  );
}
