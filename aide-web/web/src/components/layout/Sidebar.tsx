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
  Radio,
  Inbox,
  Sparkles,
  Network,
} from "lucide-react";

interface NavItem {
  to: string;
  label: string;
  icon: typeof Activity;
}

interface NavGroup {
  label: string;
  items: NavItem[];
}

// Groups are functional: Project (overview), Knowledge (durable memory
// surface), Coordination (live agent activity), Code (codebase intel),
// Telemetry (raw observability). Adjust here when adding pages.
const navGroups: NavGroup[] = [
  {
    label: "Project",
    items: [{ to: "status", label: "Status", icon: Activity }],
  },
  {
    label: "Knowledge",
    items: [
      { to: "memories", label: "Memories", icon: Brain },
      { to: "decisions", label: "Decisions", icon: GitFork },
      { to: "instincts", label: "Instincts", icon: Sparkles },
    ],
  },
  {
    label: "Code",
    items: [
      { to: "code", label: "Code", icon: Code2 },
      { to: "findings", label: "Findings", icon: AlertTriangle },
      { to: "survey", label: "Survey", icon: Map },
    ],
  },
  {
    label: "Coordination",
    items: [
      { to: "tasks", label: "Tasks", icon: ListTodo },
      { to: "messages", label: "Messages", icon: MessageSquare },
      { to: "state", label: "State", icon: Database },
      { to: "swarm", label: "Swarm", icon: Network },
    ],
  },
  {
    label: "Telemetry",
    items: [
      { to: "tokens", label: "Tokens", icon: Coins },
      { to: "observe", label: "Observe", icon: Radio },
      { to: "injections", label: "Injections", icon: Inbox },
    ],
  },
];

interface SidebarProps {
  project: string;
}

export function Sidebar({ project }: SidebarProps) {
  return (
    <aside className="w-[170px] shrink-0 pr-3 border-r border-aide-border mr-6 sticky top-16 pt-1">
      {navGroups.map((group) => (
        <div key={group.label} className="mb-3 last:mb-0">
          <div className="px-2.5 mb-1 text-[10px] font-semibold uppercase tracking-wider text-aide-text-dim">
            {group.label}
          </div>
          {group.items.map(({ to, label, icon: Icon }) => (
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
        </div>
      ))}
    </aside>
  );
}
