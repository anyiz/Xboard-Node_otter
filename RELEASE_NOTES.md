# Xboard-Node Otter v1.13-otter.2

This is the first release of the `anyiz/Xboard-Node_otter` fork. It includes strict, panel-authorized public-IP device leases shared across all landing nodes, plus TCP PROXY Protocol support for recovering client IPs through Flux relay nodes.

Assets include Linux AMD64 and ARM64 builds of `xboard-node` and `xbctl`, the installer, and `SHA256SUMS`. The installer downloads binaries from this fork's GitHub Releases by default.

The matching Xboard panel API and Flux relay changes must be deployed before enabling strict leases or PROXY Protocol on production nodes. Machine mode installation continues to use the panel-generated `--panel`, `--token`, and `--machine-id` values.

Install from this fork:

```bash
curl -fsSL https://raw.githubusercontent.com/anyiz/Xboard-Node_otter/dev/install.sh | sudo bash -s -- --mode machine --panel 'https://panel.example.com' --token 'YOUR_MACHINE_TOKEN' --machine-id 1
```
