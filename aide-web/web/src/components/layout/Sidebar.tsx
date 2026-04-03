import { NavLink } from "react-router-dom";
import { cn } from "@/lib/utils";
import {
  Activity,
  Brain,
  GitFork,
  ListTodo,
  MessageSquare,
  Database,
  Code2,
  AlertTriangle,
  Map,
  Coins,
} from "lucide-react";

const navItems = [
  { to: "status", label: "Status", icon: Activity },
  { to: "memories", label: "Memories", icon: Brain },
  { to: "decisions", label: "Decisions", icon: GitFork },
  { to: "tasks", label: "Tasks", icon: ListTodo },
  { to: "messages", label: "Messages", icon: MessageSquare },
  { to: "state", label: "State", icon: Database },
  { to: "code", label: "Code", icon: Code2 },
  { to: "findings", label: "Findings", icon: AlertTriangle },
  { to: "survey", label: "Survey", icon: Map },
  { to: "tokens", label: "Tokens", icon: Coins },
];

interface SidebarProps {
  project: string;
}

export function Sidebar({ project }: SidebarProps) {
  return (
    <aside className="w-[170px] shrink-0 pr-3 border-r border-aide-border mr-6 sticky top-16 pt-1">
      {navItems.map(({ to, label, icon: Icon }) => (
        <NavLink
          key={to}
          to={`/instances/${encodeURIComponent(project)}/${to}`}
          className={({ isActive }) =>
            cn(
              "flex items-center gap-2 px-2.5 py-1.5 mb-0.5 rounded-sm text-xs font-normal transition-all",
              isActive
                ? "bg-aide-accent/10 text-aide-accent font-semibold"
                : "text-aide-text-muted hover:bg-aide-accent/5 hover:text-aide-text"
            )
          }
        >
          <Icon className="w-3.5 h-3.5" />
          {label}
        </NavLink>
      ))}
    </aside>
  );
}
