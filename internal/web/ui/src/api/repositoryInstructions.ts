import { folderRequest } from "./localFolder";
export type InstructionTarget = { workspaceId: string; folderRevision: number; settingsRevision: number; sessionId?: string; turnId?: string };
export type InstructionSnapshot = {
 workspaceId: string; sessionId?: string; turnId?: string;
 folder: {path: string; revision: number}; settings: {enabled: boolean; revision: number};
 status: "loaded"|"disabled"|"unattached"|"missing"|"error";
 file?: string; text?: string; sha256?: string; detail?: string; capturedAt: string; prepared?: boolean;
};
export function readInstructions(target: InstructionTarget, signal?: AbortSignal) {
 const action=target.turnId?"snapshot":"preview";
 const {workspaceId,folderRevision,sessionId,turnId}=target;
 return folderRequest<InstructionSnapshot>(`/api/repository-instructions/${action}`,{workspaceId,folderRevision,sessionId,turnId},signal);
}
