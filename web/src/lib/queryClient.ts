import { QueryClient } from "@tanstack/react-query";
import { ApiError } from "./api";

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 10_000,
      refetchOnWindowFocus: false,
      retry: (count, err) =>
        !(err instanceof ApiError && (err.status === 401 || err.status === 403)) && count < 2,
    },
  },
});
