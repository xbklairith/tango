import { useAuth } from "@/lib/auth";

export default function DashboardPage() {
  const { user } = useAuth();
  const activeSquad = user?.squads[0];

  if (!activeSquad) {
    return (
      <div className="flex flex-col items-center justify-center py-20">
        <h2 className="text-xl font-semibold">Welcome to Ari</h2>
        <p className="mt-2 text-muted-foreground">
          Create your first squad to get started.
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-xl font-semibold">{activeSquad.squadName}</h2>
        <p className="text-sm text-muted-foreground">Squad overview</p>
      </div>
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
        <div className="rounded-lg border p-4">
          <p className="text-sm text-muted-foreground">Agents</p>
          <p className="text-2xl font-bold">-</p>
        </div>
        <div className="rounded-lg border p-4">
          <p className="text-sm text-muted-foreground">Issues</p>
          <p className="text-2xl font-bold">-</p>
        </div>
        <div className="rounded-lg border p-4">
          <p className="text-sm text-muted-foreground">Projects</p>
          <p className="text-2xl font-bold">-</p>
        </div>
        <div className="rounded-lg border p-4">
          <p className="text-sm text-muted-foreground">Goals</p>
          <p className="text-2xl font-bold">-</p>
        </div>
      </div>
    </div>
  );
}
