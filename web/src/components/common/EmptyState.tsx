import type { ReactNode } from "react";

export function EmptyState({
  icon,
  title,
  children,
}: {
  icon?: ReactNode;
  title: string;
  children?: ReactNode;
}) {
  return (
    <div className="flex flex-col items-center gap-2 px-4 py-12 text-center text-muted-foreground">
      {icon ? <div className="opacity-50">{icon}</div> : null}
      <h3 className="text-sm font-medium text-foreground/80">{title}</h3>
      {children ? <p className="max-w-sm text-sm leading-relaxed">{children}</p> : null}
    </div>
  );
}
