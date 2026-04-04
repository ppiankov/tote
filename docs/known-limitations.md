# Known Limitations

1. **kubelet image limit**: The kubelet reports at most 50 images per node by default (`--node-status-max-images=50`). tote agents bypass this limit by querying containerd directly.

2. **Tag-only images require multi-step resolution**: If your image reference uses a tag (`:latest`, `:v1.2.3`) instead of a digest (`@sha256:...`), tote resolves it through a 3-step chain: (1) Node.Status.Images from the kubelet, (2) agent containerd query, (3) registry v2 API lookup (opt-in via `--registry-resolve`). Without `--registry-resolve`, tag-only images with no cached digest remain not actionable. With `--registry-resolve`, the controller queries source registries to resolve the tag to a digest, then checks whether any node has that digest cached. Images that exist in the registry but were never pulled to any node are still unsalvageable — tote emits an `ImageResolvedUncached` event in this case.

3. **`imagePullPolicy: Always` blocks salvage**: kubelet will always contact the registry even if the image exists locally. Salvage only works with `imagePullPolicy: IfNotPresent` (the default for tagged images). Pods using `:latest` (which defaults to `Always`) cannot be salvaged.

4. **Agent requires root access**: The tote agent DaemonSet runs as root to access the containerd socket. The controller does not require root.
