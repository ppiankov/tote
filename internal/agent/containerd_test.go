package agent

// Ensure fakes satisfy the interface at compile time.
var (
	_ ImageStore = (*FakeImageStore)(nil)
	_ ImageStore = (*FailingImageStore)(nil)
	_ ImageStore = (*ContainerdStore)(nil)
)
