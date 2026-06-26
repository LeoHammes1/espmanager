import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, ApiError } from "@/lib/api";

export function useDeployActions() {
  const qc = useQueryClient();
  const invalidate = () => {
    void qc.invalidateQueries({ queryKey: ["overview"] });
    void qc.invalidateQueries({ queryKey: ["deploys"] });
  };
  const fail = (verb: string) => (e: unknown) =>
    toast.error(e instanceof ApiError ? e.message : `${verb} failed.`);

  const resume = useMutation({
    mutationFn: (id: string) => api.post<void>(`/api/deploys/${id}/resume`),
    onSuccess: () => {
      toast.success("Deploy resumed.");
      invalidate();
    },
    onError: fail("Resume"),
  });
  const cancel = useMutation({
    mutationFn: (id: string) => api.post<void>(`/api/deploys/${id}/cancel`),
    onSuccess: () => {
      toast.success("Deploy cancelled.");
      invalidate();
    },
    onError: fail("Cancel"),
  });
  return { resume, cancel };
}
