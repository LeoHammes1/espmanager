import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter } from "react-router-dom";
import { queryClient } from "@/lib/queryClient";
import { AuthProvider } from "@/auth/AuthProvider";
import { Toaster } from "@/components/ui/sonner";
import { App } from "@/App";
import "./index.css";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <AuthProvider>
          <App />
        </AuthProvider>
      </BrowserRouter>
      <Toaster position="bottom-right" richColors />
    </QueryClientProvider>
  </StrictMode>,
);
