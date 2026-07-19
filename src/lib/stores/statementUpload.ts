import { writable } from "svelte/store";

export type UploadStatus =
  | "uploading"
  | "queued"
  | "done"
  | "failed"
  | "unknown"
  | "error"
  | "timeout";

export interface UploadRow {
  id: number;
  name: string; // stored filename returned by the agent
  size: number;
  status: UploadStatus;
  error?: string; // upload-time error message
  summary?: { matched: number; missing: number; extra: number; period: string };
  startedAt: number;
}

// Module-level so the session list survives closing/reopening the modal.
export const uploadRows = writable<UploadRow[]>([]);
let nextId = 1;
export function newRowId(): number {
  return nextId++;
}
