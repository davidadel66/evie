export type ChatModel = { id: string; name: string; provider: string };
export type ModelSelection = { model: string; revision: number };
export type ModelCatalog = ModelSelection & { models: ChatModel[]; problem?: string };

async function post<T>(path: string, body: unknown, signal?: AbortSignal): Promise<T> {
  const response = await fetch(path, {
    method: "POST", headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body), signal,
  });
  const value = await response.json();
  if (!response.ok) throw new Error(value.error ?? "Model request failed");
  return value as T;
}

export function listModels(sessionId: string, signal?: AbortSignal): Promise<ModelCatalog> {
  return post("/api/models/list", { sessionId }, signal);
}

export function selectModel(sessionId: string, model: string, revision: number): Promise<ModelSelection> {
  return post("/api/models/select", { sessionId, model, revision });
}
