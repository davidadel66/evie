import type { InstructionSnapshot } from "../api/repositoryInstructions";
export function instructionLabel(snapshot?: InstructionSnapshot) {
 if(!snapshot)return "Repository instructions";
 switch(snapshot.status){case "loaded":return snapshot.file ?? "Repository instructions";case "disabled":return "Instructions off";case "missing":return "No instructions";case "unattached":return "No folder attached";case "error":return "Instructions unavailable";}
}
