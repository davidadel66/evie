// The "Always applies" switch on a memory approval card records the owner's
// choice in the session store, so the card's Approve button and the Y
// shortcut approve the same exact proposal.

import { createContext } from "react";

export type AlwaysAppliesSetter = (reqId: string, alwaysApplies: boolean) => void;

export const AlwaysAppliesContext = createContext<AlwaysAppliesSetter | undefined>(undefined);
