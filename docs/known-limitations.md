# Known Limitations

1. **kubelet image limit**: The kubelet reports at most 50 images per node by default (`--node-status-max-images=50`). tote agents bypass this limit by querying containerd directly.

2. **Tag-only images are not actionable**: If your image reference uses a tag (`:latest`, `:v1.2.3`) instead of a digest (`@sha256:...`), tote cannot verify identity across nodes. When agents are deployed, tote can resolve tags via containerd as a fallback.

3. **`imagePullPolicy: Always` blocks salvage**: kubelet will always contact the registry even if the image exists locally. Salvage only works with `imagePullPolicy: IfNotPresent` (the default for tagged images). Pods using `:latest` (which defaults to `Always`) cannot be salvaged.

4. **Agent requires root access**: The tote agent DaemonSet runs as root to access the containerd socket. The controller does not require root.
