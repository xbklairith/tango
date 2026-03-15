import { NavLink } from "react-router";
import { useAuth } from "@/lib/auth";
import { useActiveSquad } from "@/lib/active-squad";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  LayoutDashboard,
  Bot,
  CircleDot,
  FolderKanban,
  Target,
  MessageSquare,
  Inbox,
  Workflow,
  LogOut,
} from "lucide-react";
import { InboxBadge } from "@/features/inbox/inbox-badge";

interface SidebarProps {
  onNavigate?: () => void;
}

const staticNavItems: { to: string; icon: any; label: string; end?: boolean }[] = [];

function SquadSelector() {
  const { user } = useAuth();
  const { activeSquadId, setActiveSquadId } = useActiveSquad();
  const squads = user?.squads ?? [];

  if (squads.length === 0) return null;

  if (squads.length === 1) {
    return (
      <p className="px-3 py-1 text-sm font-medium truncate">
        {squads[0]!.squadName}
      </p>
    );
  }

  return (
    <div className="px-2">
      <Select value={activeSquadId ?? ""} onValueChange={(v) => { if (v) setActiveSquadId(v); }}>
        <SelectTrigger className="w-full text-sm truncate">
          <SelectValue placeholder="Select squad">
            {squads.find((s) => s.squadId === activeSquadId)?.squadName ?? "Select squad"}
          </SelectValue>
        </SelectTrigger>
        <SelectContent alignItemWithTrigger={false}>
          {squads.map((squad) => (
            <SelectItem key={squad.squadId} value={squad.squadId}>
              {squad.squadName}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

export function Sidebar({ onNavigate }: SidebarProps) {
  const { user, logout } = useAuth();
  const { activeSquadId } = useActiveSquad();

  const squadNavItems = activeSquadId
    ? [
        { to: "/", icon: LayoutDashboard, label: "Dashboard", end: true },
        { to: `/squads/${activeSquadId}/agents`, icon: Bot, label: "Agents" },
        { to: `/squads/${activeSquadId}/conversations`, icon: MessageSquare, label: "Conversations" },
        { to: `/squads/${activeSquadId}/issues`, icon: CircleDot, label: "Issues" },
        { to: `/squads/${activeSquadId}/inbox`, icon: Inbox, label: "Inbox", hasBadge: true },
        { to: `/squads/${activeSquadId}/projects`, icon: FolderKanban, label: "Projects" },
        { to: `/squads/${activeSquadId}/goals`, icon: Target, label: "Goals" },
        { to: "/pipelines", icon: Workflow, label: "Pipelines" },
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
            <div className="flex items-center justify-between px-3 py-2">
              <span className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">Squad</span>
              <NavLink to="/squads" onClick={onNavigate} className="text-xs text-muted-foreground hover:text-foreground">
                Manage
              </NavLink>
            </div>
            <SquadSelector />
            {squadNavItems.map((item) => (
              <NavLink
                key={item.to}
                to={item.to}
                end={"end" in item ? (item as any).end : undefined}
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
                {"hasBadge" in item && item.hasBadge && activeSquadId && (
                  <InboxBadge squadId={activeSquadId} />
                )}
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
