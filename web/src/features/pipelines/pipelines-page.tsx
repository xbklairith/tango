import { useState } from "react";
import { Link } from "react-router";
import { useActiveSquad } from "@/lib/active-squad";
import { formatDate } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Plus } from "lucide-react";
import { usePipelines } from "./api";
import { PipelineDialog } from "./pipeline-dialog";

type FilterMode = "all" | "active" | "inactive";

export default function PipelinesPage() {
  const { activeSquadId } = useActiveSquad();
  const [createOpen, setCreateOpen] = useState(false);
  const [filterMode, setFilterMode] = useState<FilterMode>("all");

  const filters =
    filterMode === "all"
      ? undefined
      : { isActive: filterMode === "active" };

  const { data, isLoading } = usePipelines(activeSquadId, filters);
  const pipelines = data?.data;

  if (isLoading) {
    return (
      <div className="animate-pulse space-y-4">
        {Array.from({ length: 3 }, (_, i) => (
          <div key={i} className="h-16 rounded-md bg-muted" />
        ))}
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold">Pipelines</h2>
        <Button size="sm" onClick={() => setCreateOpen(true)}>
          <Plus className="h-4 w-4 mr-1" />
          Create Pipeline
        </Button>
      </div>

      <div className="flex gap-1">
        {(["all", "active", "inactive"] as FilterMode[]).map((mode) => (
          <Button
            key={mode}
            size="sm"
            variant={filterMode === mode ? "default" : "outline"}
            onClick={() => setFilterMode(mode)}
          >
            {mode.charAt(0).toUpperCase() + mode.slice(1)}
          </Button>
        ))}
      </div>

      <div className="rounded-md border">
        <table className="w-full">
          <thead>
            <tr className="border-b bg-muted/50">
              <th className="px-4 py-3 text-left text-sm font-medium">
                Name
              </th>
              <th className="px-4 py-3 text-left text-sm font-medium">
                Status
              </th>
              <th className="px-4 py-3 text-left text-sm font-medium">
                Description
              </th>
              <th className="px-4 py-3 text-left text-sm font-medium">
                Created
              </th>
            </tr>
          </thead>
          <tbody>
            {pipelines?.length === 0 && (
              <tr>
                <td
                  colSpan={4}
                  className="px-4 py-8 text-center text-sm text-muted-foreground"
                >
                  No pipelines found
                </td>
              </tr>
            )}
            {pipelines?.map((pipeline) => (
              <tr key={pipeline.id} className="border-b last:border-0">
                <td className="px-4 py-3">
                  <Link
                    to={`/pipelines/${pipeline.id}`}
                    className="text-sm font-medium hover:underline"
                  >
                    {pipeline.name}
                  </Link>
                </td>
                <td className="px-4 py-3">
                  <span
                    className={`inline-flex rounded-full px-2 py-1 text-xs font-medium ${
                      pipeline.isActive
                        ? "bg-green-100 text-green-800"
                        : "bg-gray-100 text-gray-800"
                    }`}
                  >
                    {pipeline.isActive ? "Active" : "Inactive"}
                  </span>
                </td>
                <td className="px-4 py-3 text-sm text-muted-foreground truncate max-w-xs">
                  {pipeline.description}
                </td>
                <td className="px-4 py-3 text-sm text-muted-foreground">
                  {formatDate(pipeline.createdAt)}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {activeSquadId && (
        <PipelineDialog
          open={createOpen}
          onOpenChange={setCreateOpen}
          squadId={activeSquadId}
        />
      )}
    </div>
  );
}
