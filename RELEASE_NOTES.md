# Xboard-Node Otter v1.13-otter.3

Adds panel-managed strict public-IP lease settings for machine-mode nodes. The panel can supply an enabled flag, trusted proxy CIDRs and policy revision per node. A policy change stops the old node service before starting its replacement; other nodes on the machine keep running. Missing policy preserves local configuration.

Nodes report `X-Device-Policy: v1` capability and the running policy revision. The matching Xboard panel waits for a fresh applied revision before issuing protected credentials. Managed nodes can start with no users to complete this acknowledgement safely. Invalid or unrestricted trust ranges are rejected. Failed node services are retried during discovery.

Use with the matching Xboard visual policy UI. Flux PROXY v2 and host firewall remain separately configured; this release does not modify either. Single-node mode continues using local configuration. Policy changes interrupt connections on that node. No existing credential allowlist is expanded automatically.

Includes Linux amd64/arm64 binaries and installer. Upgrade only the isolated test services first, preserving machine identity and existing configuration.
