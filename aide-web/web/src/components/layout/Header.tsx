import { useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { api } from "@/lib/api";
import type { InstanceInfo } from "@/lib/types";
import { StatusBadge } from "../shared/StatusBadge";

export function Header() {
  const [instances, setInstances] = useState<InstanceInfo[]>([]);
  const [version, setVersion] = useState<string>("");
  const { project: activeProject } = useParams<{ project: string }>();

  useEffect(() => {
    api.getVersion().then(setVersion).catch(() => {});
  }, []);

  useEffect(() => {
    const fetch = () =>
      api
        .listInstances()
        .then((list) =>
          setInstances(
            [...list].sort((a, b) =>
              a.project_name.localeCompare(b.project_name)
            )
          )
        )
        .catch(console.error);
    fetch();
    const id = setInterval(fetch, 5000);
    return () => clearInterval(id);
  }, []);

  return (
    <header className="w-full border-b border-aide-border mb-6 bg-aide-bg/95 backdrop-blur-md sticky top-0 z-50">
      <nav className="flex items-center justify-between min-h-12 px-6">
        <Link
          to="/"
          className="font-mono font-bold text-sm text-aide-text hover:text-aide-accent transition-colors"
        >
          aide-web
          {version && (
            <span className="ml-1.5 text-[10px] font-normal text-aide-text-dim">
              v{version}
            </span>
          )}
        </Link>

        <div className="flex items-center gap-0.5">
          {instances.map((inst) => {
            const isActive = activeProject === inst.project_name;
            return (
              <Link
                key={inst.project_name}
                to={`/instances/${encodeURIComponent(inst.project_name)}/status`}
                className={`
                  inline-flex items-center gap-1.5 px-2.5 py-1.5 rounded-sm
                  text-xs font-medium transition-all
                  ${
                    isActive
                      ? "bg-aide-accent/15 text-aide-accent ring-1 ring-aide-accent/30"
                      : inst.status === "connected"
                        ? "bg-aide-green/10 text-aide-green"
                        : inst.status === "connecting"
                          ? "text-aide-yellow"
                          : "text-aide-text-dim opacity-40"
                  }
                  hover:text-aide-text hover:bg-aide-surface-hover
                `}
              >
                <StatusBadge status={inst.status} />
                {inst.project_name}
              </Link>
            );
          })}
          <Link
            to="/search"
            className="px-2.5 py-1.5 rounded-sm text-xs font-medium text-aide-text-muted hover:text-aide-text hover:bg-aide-surface-hover transition-all"
          >
            Search
          </Link>
        </div>
      </nav>
    </header>
  );
}
