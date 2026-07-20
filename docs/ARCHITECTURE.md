# Syntax
# PathWeaver — Go + Vue rewrite

See [README.md](../README.md) and [PathWeaver 最终开发计划.md](../PathWeaver%20最终开发计划.md).

## Stack

- Controller / Agent / netd / updater: Go
- Web UI: Vue 3 + TypeScript + Vite + Cytoscape.js
- Storage: SQLite
- Control plane: HTTP + WebSocket (admin); bootstrap HTTP for enrollment; DesiredState over agent pull

## Processes

Documented in packaging/systemd/.
