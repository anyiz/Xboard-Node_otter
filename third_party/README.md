# Pinned kernel source

These two directories are source copies of the upstream forks already referenced by this project's `go.mod` before the strict device lease change:

- `sing-box`: `github.com/cedar2025/sing-box` revision `2e665cb7e295`, Go module version `v1.14.0-alpha.2.0.20260316103356-2e665cb7e295`. Local changes add trusted, required PROXY parsing before TLS/user authentication and connection admission before routing.
- `xray-core`: `github.com/cedar2025/Xray-core` revision `f47935539965`, Go module version `v0.0.0-20260409213332-f47935539965`. Local changes add trusted, required PROXY parsing before protocol handling.

Their original `LICENSE` files are retained. The root `go.mod` replaces the imported modules with these directories so Docker and normal builds use the same patched code. When updating either dependency, reapply and test the strict PROXY and admission changes before changing the module version.
