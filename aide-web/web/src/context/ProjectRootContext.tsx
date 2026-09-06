import { createContext, useContext, type ReactNode } from "react";
import { api } from "@/lib/api";
import { useApi } from "@/hooks/use-api";

const ProjectRootContext = createContext<string | undefined>(undefined);

export function ProjectRootProvider({
  project,
  children,
}: {
  project?: string;
  children: ReactNode;
}) {
  const { data: instances } = useApi(
    () => (project ? api.listInstances() : Promise.resolve([])),
    [project],
  );
  const exact = instances?.find((instance) => instance.slug === project);
  const matches =
    instances?.filter((instance) => instance.project_name === project) ?? [];
  const connected = matches.filter(
    (instance) => instance.status === "connected",
  );
  const instance =
    exact ??
    (matches.length === 1
      ? matches[0]
      : connected.length === 1
        ? connected[0]
        : undefined);
  return (
    <ProjectRootContext.Provider value={instance?.project_root}>
      {children}
    </ProjectRootContext.Provider>
  );
}

export function useProjectRoot() {
  return useContext(ProjectRootContext);
}
