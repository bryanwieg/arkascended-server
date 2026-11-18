# Add a Backend Method

Adding a method to `ark.ContainerEngine` or `ark.ComposeService`:

1. Add the signature to the interface in `app/pkg/ark/engine.go` or
   `app/pkg/ark/compose_service.go`. `ctx context.Context` goes first. Map native
   errors to the sentinels in `app/pkg/ark/errors.go` (`ErrNotFound`,
   `ErrConflict`, `ErrTimeout`) and document which ones the method returns.
2. Implement it on `SDKEngine` or `SDKCompose` in `app/pkg/ark/sdkbackend/`.
3. Add a `<Method>Fn` field and the delegating method to `MockEngine` or
   `MockCompose` in `app/pkg/ark/arktest/mocks.go`. If the field is nil, return a
   zero value. The compile-time checks there and in `sdkbackend/` fail until
   both are done.
4. Add a `t.Run` subtest to `TestContainerEngine` (`conformance/engine.go`) or
   `TestComposeService` (`conformance/compose.go`).
5. Run the conformance suite. It covers Docker and rootless Podman, and skips
   a runtime that is not reachable:

   ```bash
   cd app/pkg/ark && go test -tags integration -v ./sdkbackend_test/
   ```

6. Update the interface list in the [backend reference](../reference/backends.md#interfaces).
