import { useCallback, useEffect, useState } from "react";
import { changesAPI, type ChangeList, type ChangesAPI } from "../api/changes";

/**
 * A Workspace's Changes, read when it opens and every 30 seconds: the
 * header's badge and whether the Changes tab shows at all come from it.
 */
export function useChangeListing(workspaceId: string, api: ChangesAPI = changesAPI) {
  const [listing, setListing] = useState<ChangeList>();
  const reload = useCallback(() => {
    api.list(workspaceId).then(setListing, () => undefined);
  }, [api, workspaceId]);
  useEffect(() => {
    setListing(undefined);
    reload();
    const timer = window.setInterval(reload, 30_000);
    return () => window.clearInterval(timer);
  }, [reload]);
  return {listing, reload};
}
