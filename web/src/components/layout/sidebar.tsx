import { NavLink } from "react-router";
import { useAuth } from "@/lib/auth";
import {
  LayoutDashboard,
  Users,
  Bot,
  CircleDot,
  FolderKanban,
  Target,
  LogOut,
} from "lucide-react";

interface SidebarProps {
  onNavigate?: () => void;
}

const staticNavItems = [
  { to: "/", icon: LayoutDashboard, label: "Dashboard", end: true },
  { to: "/squads", icon: Users, label: "Squads", end: false },
];

export function Sidebar({ onNavigate }: SidebarProps) {
  const { user, logout } = useAuth();
  const activeSquadId = user?.squads?.[0]?.squadId;

  const squadNavItems = activeSquadId
    ? [
        { to: `/squads/${activeSquadId}/agents`, icon: Bot, label: "Agents" },
        { to: `/squads/${activeSquadId}/issues`, icon: CircleDot, label: "Issues" },
        { to: `/squads/${activeSquadId}/projects`, icon: FolderKanban, label: "Projects" },
        { to: `/squads/${activeSquadId}/goals`, icon: Target, label: "Goals" },
      ]
    : [];

  return (
    <div className="flex h-full flex-col">
      <div className="flex h-14 items-center px-4 font-semibold text-lg border-b">
        Ari
      </div>

      <nav className="flex-1 space-y-1 px-2 py-3">
        {staticNavItems.map((item) => (
          <NavLink
            key={item.to}
            to={item.to}
            end={item.end}
            onClick={onNavigate}
            className={({ isActive }) =>
              `flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors ${
                isActive
                  ? "bg-primary text-primary-foreground"
                  : "text-muted-foreground hover:bg-accent hover:text-accent-foreground"
              }`
            }
          >
            <item.icon className="h-4 w-4" />
            {item.label}
          </NavLink>
        ))}

        {squadNavItems.length > 0 && (
          <>
            <div className="px-3 py-2 text-xs font-semibold text-muted-foreground uppercase tracking-wider">
              Squad
            </div>
            {squadNavItems.map((item) => (
              <NavLink
                key={item.to}
                to={item.to}
                onClick={onNavigate}
                className={({ isActive }) =>
                  `flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors ${
                    isActive
                      ? "bg-primary text-primary-foreground"
                      : "text-muted-foreground hover:bg-accent hover:text-accent-foreground"
                  }`
                }
              >
                <item.icon className="h-4 w-4" />
                {item.label}
              </NavLink>
            ))}
          </>
        )}
      </nav>

      <div className="border-t p-3">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2 min-w-0">
            <div className="flex h-8 w-8 items-center justify-center rounded-full bg-primary text-primary-foreground text-sm font-medium">
              {user?.displayName?.charAt(0)?.toUpperCase() ?? "?"}
            </div>
            <span className="truncate text-sm">{user?.displayName ?? user?.email}</span>
          </div>
          <button
            onClick={logout}
            className="text-muted-foreground hover:text-foreground"
            title="Logout"
          >
            <LogOut className="h-4 w-4" />
          </button>
        </div>
      </div>
    </div>
  );
}
