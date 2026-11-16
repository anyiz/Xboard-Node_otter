# Xboard-Node Otter v1.13-otter.4

Fixes proxy kernels remaining stopped after an empty user list receives valid users. Full and incremental user updates now prepare the new user/limiter state before starting a stopped kernel; startup failures restore the previous state.

Configuration updates keep an empty-user kernel running so panel-managed strict lease policies can report a healthy applied revision before protected users are released. Empty user lists do not authorize any credentials. Strict mode, trusted proxy checks, device limits and lease validation remain enforced.

Regression tests cover full and incremental recovery, failed-start rollback and empty-user configuration updates. This promotes the v1.13-otter.3-empty-user-fix patch already verified on live machine-mode nodes to an official release.

Includes Linux amd64/arm64 node and xbctl binaries, SHA256SUMS and install.sh. Install with --version v1.13-otter.4; preserve existing machine identity and configuration. Restarting the node briefly interrupts its connections.
