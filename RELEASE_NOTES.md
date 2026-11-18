# Xboard-Node Otter v1.13-otter.5

Adds the panel-managed `compatible` device policy mode. Trusted TCP relays must send a valid PROXY Protocol header so the node sees the real client IP, while untrusted peers remain usable as ordinary direct connections. The existing per-user device limiter and cross-node IP snapshots continue to apply without requiring strict Redis leases.

The policy wire capability is now `v2`. Panels can safely keep `v1` nodes on strict mode and only offer compatibility mode after a new node reports `v2`. Xray and sing-box both enforce trusted relay CIDRs; unrestricted `/0` trust is rejected. Strict mode behavior remains unchanged.

Also includes the validated `XBOARD_ACME_DNS_RESOLVERS` override for DNS-01 propagation checks. This permits explicit resolvers without disabling TXT-record verification.

Includes Linux amd64/arm64 node and xbctl binaries, SHA256SUMS and install.sh. Install with `--version v1.13-otter.5`; preserve existing machine identity and configuration. Restarting the node briefly interrupts its connections.
