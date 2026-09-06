import { Outlet, useParams } from "react-router-dom";
import { Header } from "./Header";
import { Sidebar } from "./Sidebar";
import { ProjectRootProvider } from "@/context/ProjectRootContext";

export function AppLayout() {
  const { project } = useParams();

  return (
    <div className="min-h-screen flex flex-col">
      <Header />
      <main className="flex-1 w-full max-w-full px-3 sm:px-6 pb-8">
        <ProjectRootProvider project={project}>
        {project ? (
          <div className="flex flex-col md:flex-row items-start gap-0">
            <Sidebar project={project} />
            <div className="w-full flex-1 min-w-0">
              <Outlet />
            </div>
          </div>
        ) : (
          <Outlet />
        )}
        </ProjectRootProvider>
      </main>
    </div>
  );
}
