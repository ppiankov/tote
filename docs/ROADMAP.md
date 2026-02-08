# Roadmap

## v0.2.0 cleanup debt

- [ ] Add `make proto` target to Makefile (currently manual `protoc` invocation)
- [ ] Add `helm lint` step to CI pipeline
- [ ] Improve test coverage for `agent` and `transfer` packages (33% — untested paths require a live containerd socket)

## v0.3 — Registry push

- [ ] Push salvaged images back to a registry (not just node-to-node transfer)
- [ ] Image size limits to prevent transferring oversized images

## v1.0 — Production hardening

- [ ] mTLS between agents (currently plaintext gRPC within cluster network)
- [ ] Leader election for the controller
- [ ] CRDs for salvage tracking (replace pod annotations)
- [ ] Pod deletion / fast-recover (opt-in, requires explicit user consent)

## Out of scope

These are explicitly not planned:

- ML-based prediction of pull failures
- Multi-cluster image transfer
- Registry mirroring or caching proxy
- Automatic image rebuilds
