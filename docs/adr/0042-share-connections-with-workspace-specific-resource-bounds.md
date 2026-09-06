# Share connections with Workspace-specific resource bounds

One Kernel-owned Account Connection may be referenced by multiple Workspaces
without duplicating credentials. Each Workspace independently permits the
resources reachable through that connection. Removing a shared Connection ID
or resource from one Workspace revokes only that Workspace's access and does
not disconnect other consumers.
