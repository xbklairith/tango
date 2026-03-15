import { useState } from "react";
import { useParams, Link } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { queryKeys } from "@/lib/query";
import { formatDateTime } from "@/lib/utils";
import type { Agent } from "@/types/agent";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  ArrowLeft,
  Plus,
  Trash2,
} from "lucide-react";
import {
  usePipeline,
  useUpdatePipeline,
  useDeletePipeline,
  useCreateStage,
  useDeleteStage,
} from "./api";
import { PipelineDialog } from "./pipeline-dialog";
import { useNavigate } from "react-router";

export default function PipelineDetailPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { data: pipeline, isLoading } = usePipeline(id);

  const [editOpen, setEditOpen] = useState(false);
  const [stageForm, setStageForm] = useState({
    name: "",
    position: "",
    assignedAgentId: "",
  });
  const [stageErrors, setStageErrors] = useState<Record<string, string>>({});

  const updatePipeline = useUpdatePipeline();
  const deletePipeline = useDeletePipeline();
  const createStage = useCreateStage();
  const deleteStage = useDeleteStage();

  const { data: agents } = useQuery({
    queryKey: queryKeys.agents.list(pipeline?.squadId ?? ""),
    queryFn: () =>
      api.get<Agent[]>(`/agents?squadId=${pipeline!.squadId}`),
    enabled: !!pipeline?.squadId,
  });

  if (isLoading) {
    return (
      <div className="animate-pulse space-y-4">
        <div className="h-8 w-48 rounded bg-muted" />
        <div className="h-32 rounded bg-muted" />
      </div>
    );
  }

  if (!pipeline) return <p>Pipeline not found</p>;

  const stages = [...(pipeline.stages ?? [])].sort(
    (a, b) => a.position - b.position,
  );

  function handleCreateStage() {
    const errors: Record<string, string> = {};
    if (!stageForm.name.trim()) errors.name = "Name is required";
    const pos = Number(stageForm.position);
    if (!stageForm.position || isNaN(pos) || pos < 1)
      errors.position = "Position must be at least 1";
    if (Object.keys(errors).length > 0) {
      setStageErrors(errors);
      return;
    }

    createStage.mutate(
      {
        pipelineId: pipeline!.id,
        data: {
          name: stageForm.name.trim(),
          position: pos,
          assignedAgentId: stageForm.assignedAgentId || undefined,
        },
      },
      {
        onSuccess: () => {
          setStageForm({ name: "", position: "", assignedAgentId: "" });
          setStageErrors({});
        },
      },
    );
  }

  function handleDeleteStage(stageId: string) {
    if (!confirm("Are you sure you want to delete this stage?")) return;
    deleteStage.mutate({ id: stageId, pipelineId: pipeline!.id });
  }

  function handleDeletePipeline() {
    if (
      !confirm(
        "Are you sure you want to delete this pipeline? This cannot be undone.",
      )
    )
      return;
    deletePipeline.mutate(
      { id: pipeline!.id, squadId: pipeline!.squadId },
      { onSuccess: () => navigate("/pipelines") },
    );
  }

  function handleToggleActive() {
    updatePipeline.mutate({
      id: pipeline!.id,
      data: { isActive: !pipeline!.isActive },
    });
  }

  const nextPosition =
    stages.length > 0 ? stages[stages.length - 1]!.position + 1 : 1;

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <Link
          to="/pipelines"
          className="flex items-center gap-1 hover:text-foreground"
        >
          <ArrowLeft className="h-4 w-4" />
          Pipelines
        </Link>
      </div>

      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <h2 className="text-xl font-semibold">{pipeline.name}</h2>
          <span
            className={`inline-flex rounded-full px-2 py-1 text-xs font-medium ${
              pipeline.isActive
                ? "bg-green-100 text-green-800"
                : "bg-gray-100 text-gray-800"
            }`}
          >
            {pipeline.isActive ? "Active" : "Inactive"}
          </span>
        </div>
        <div className="flex gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={handleToggleActive}
            disabled={updatePipeline.isPending}
          >
            {pipeline.isActive ? "Deactivate" : "Activate"}
          </Button>
          <Button variant="outline" size="sm" onClick={() => setEditOpen(true)}>
            Edit
          </Button>
          <Button
            variant="destructive"
            size="sm"
            onClick={handleDeletePipeline}
            disabled={deletePipeline.isPending}
          >
            Delete
          </Button>
        </div>
      </div>

      {pipeline.description && (
        <div className="rounded-lg border p-4">
          <p className="text-sm">{pipeline.description}</p>
        </div>
      )}

      <p className="text-xs text-muted-foreground">
        Created {formatDateTime(pipeline.createdAt)}
      </p>

      {/* Stages Section */}
      <div className="space-y-4">
        <h3 className="text-lg font-semibold">
          Stages ({stages.length})
        </h3>

        {stages.length > 0 && (
          <div className="rounded-md border">
            <table className="w-full">
              <thead>
                <tr className="border-b bg-muted/50">
                  <th className="px-4 py-3 text-left text-sm font-medium w-16">
                    #
                  </th>
                  <th className="px-4 py-3 text-left text-sm font-medium">
                    Name
                  </th>
                  <th className="px-4 py-3 text-left text-sm font-medium">
                    Assigned Agent
                  </th>
                  <th className="px-4 py-3 text-left text-sm font-medium">
                    Description
                  </th>
                  <th className="px-4 py-3 text-right text-sm font-medium w-20">
                    Actions
                  </th>
                </tr>
              </thead>
              <tbody>
                {stages.map((stage) => {
                  const agent = agents?.find(
                    (a) => a.id === stage.assignedAgentId,
                  );
                  return (
                    <tr
                      key={stage.id}
                      className="border-b last:border-0"
                    >
                      <td className="px-4 py-3 text-sm font-mono text-muted-foreground">
                        {stage.position}
                      </td>
                      <td className="px-4 py-3 text-sm font-medium">
                        {stage.name}
                      </td>
                      <td className="px-4 py-3 text-sm text-muted-foreground">
                        {agent?.name ?? (stage.assignedAgentId ? "Unknown" : "-")}
                      </td>
                      <td className="px-4 py-3 text-sm text-muted-foreground truncate max-w-xs">
                        {stage.description ?? "-"}
                      </td>
                      <td className="px-4 py-3 text-right">
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => handleDeleteStage(stage.id)}
                          disabled={deleteStage.isPending}
                        >
                          <Trash2 className="h-4 w-4 text-destructive" />
                        </Button>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}

        {/* Add Stage Form */}
        <div className="rounded-lg border p-4 space-y-3">
          <h4 className="text-sm font-medium">Add Stage</h4>
          <div className="flex gap-3 items-end flex-wrap">
            <div className="space-y-1">
              <Label htmlFor="stage-name">Name</Label>
              <Input
                id="stage-name"
                maxLength={200}
                value={stageForm.name}
                onChange={(e) => {
                  setStageForm({ ...stageForm, name: e.target.value });
                  setStageErrors({ ...stageErrors, name: "" });
                }}
                placeholder="e.g. Code Review"
              />
              {stageErrors.name && (
                <p className="text-xs text-destructive">
                  {stageErrors.name}
                </p>
              )}
            </div>
            <div className="space-y-1">
              <Label htmlFor="stage-position">Position</Label>
              <Input
                id="stage-position"
                type="number"
                min={1}
                className="w-24"
                value={stageForm.position || String(nextPosition)}
                onChange={(e) => {
                  setStageForm({
                    ...stageForm,
                    position: e.target.value,
                  });
                  setStageErrors({ ...stageErrors, position: "" });
                }}
              />
              {stageErrors.position && (
                <p className="text-xs text-destructive">
                  {stageErrors.position}
                </p>
              )}
            </div>
            <div className="space-y-1">
              <Label>Assigned Agent</Label>
              <Select
                value={stageForm.assignedAgentId}
                onValueChange={(v) =>
                  setStageForm({
                    ...stageForm,
                    assignedAgentId: v === "__none__" ? "" : v,
                  })
                }
              >
                <SelectTrigger className="w-48">
                  <SelectValue placeholder="None" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="__none__">None</SelectItem>
                  {agents?.map((a) => (
                    <SelectItem key={a.id} value={a.id}>
                      {a.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <Button
              size="sm"
              onClick={handleCreateStage}
              disabled={createStage.isPending}
            >
              <Plus className="h-4 w-4 mr-1" />
              Add
            </Button>
          </div>
        </div>
      </div>

      <PipelineDialog
        open={editOpen}
        onOpenChange={setEditOpen}
        squadId={pipeline.squadId}
        pipeline={pipeline}
      />
    </div>
  );
}
