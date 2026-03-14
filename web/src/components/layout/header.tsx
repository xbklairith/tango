import { useLocation } from "react-router";
import { Menu } from "lucide-react";

interface HeaderProps {
  onMenuClick: () => void;
}

const pageTitles: Record<string, string> = {
  "/": "Dashboard",
  "/squads": "Squads",
};

function getPageTitle(pathname: string): string {
  if (pageTitles[pathname]) return pageTitles[pathname]!;
  if (pathname.match(/^\/squads\/[^/]+\/agents$/)) return "Agents";
  if (pathname.match(/^\/agents\/[^/]+$/)) return "Agent Detail";
  if (pathname.match(/^\/squads\/[^/]+\/issues$/)) return "Issues";
  if (pathname.match(/^\/issues\/[^/]+$/)) return "Issue Detail";
  if (pathname.match(/^\/squads\/[^/]+\/projects$/)) return "Projects";
  if (pathname.match(/^\/projects\/[^/]+$/)) return "Project Detail";
  if (pathname.match(/^\/squads\/[^/]+\/goals$/)) return "Goals";
  if (pathname.match(/^\/goals\/[^/]+$/)) return "Goal Detail";
  if (pathname.match(/^\/squads\/[^/]+$/)) return "Squad Detail";
  return "Ari";
}

export function Header({ onMenuClick }: HeaderProps) {
  const location = useLocation();
  const title = getPageTitle(location.pathname);

  return (
    <header className="flex h-14 items-center gap-4 border-b px-6">
      <button
        onClick={onMenuClick}
        className="lg:hidden text-muted-foreground hover:text-foreground"
      >
        <Menu className="h-5 w-5" />
      </button>
      <h1 className="text-lg font-semibold">{title}</h1>
    </header>
  );
}
