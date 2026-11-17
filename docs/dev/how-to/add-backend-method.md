# Add a Backend Method

How to add a method to `ark.ContainerEngine` or `ark.ComposeService`.

1. Add the method signature to the interface in `app/pkg/ark/engine.go` or
   `app/pkg/ark/compose_service.go`. Take `ctx context.Context` as the first
   argument and map "not found" errors to `ark.ErrNotFound`.
2. Implement it in the SDK backend (`app/pkg/ark/sdkbackend/`).
3. Add the method to the mocks in `app/pkg/ark/arktest/` so CLI and domain
   tests still compile.
4. Add a conformance test in `app/pkg/ark/conformance/`.
5. Run the conformance suite against Docker (and Podman, if available):

   ```bash
   cd app/pkg/ark && go test -tags integration -v ./sdkbackend_test/
   ```

6. Update the interface list in the [backend reference](../reference/backends.md).
