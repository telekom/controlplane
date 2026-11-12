---
id: 14-no-redundant-tests
description: New test cases must not duplicate existing ones; extend existing tables instead, without mixing unit, envtest and e2e scopes.
severity: LOW
complexity: 1
scope: file
applies_to:
  - "**/*_test.go"
exclude:
  - "**/suite_test.go"
  - "**/vendor/**"
context_lines: 40
---

# No redundant tests

Generated tests tend to add a fresh `It` for every change, even when the file
already asserts the same behavior, or when the new case is a copy of an
existing one with different literals. Each copy has to be maintained and
fixed separately, so the suite grows without covering anything new.

Prefer reusing what is there: add an `Entry` to an existing `DescribeTable`,
add a case to an existing table-driven loop, or fold copy-pasted `It` blocks
into one table.

Reuse stops at the scope boundary. Unit tests (mocks such as
`NewMockJanitorClient`, no API server), envtest tests (a real `k8sClient`
from `suite_test.go`) and e2e tests are different scopes. A unit case must not
be pushed into an envtest table, or an envtest case into a unit table, just
because a similar table already exists. Separate suites for separate scopes
are not redundancy.

Same `It` titles alone are not redundancy. Tests are distinct when they
exercise different behavior — a different branch, setup or assertion, as with
`CreateOrUpdate` vs `Delete` failure paths that need different preparation.
A body that is identical except for the function name and literals is still
a copy, even if the function differs: it belongs in one table.

Only judge the tests the change adds or modifies, compared with the rest of
the file. Decide from the context you are shown: if it contains no equivalent
test, or no test of a different scope, answer `PASS` — do not answer
`UNCLEAR` because such a test *might* exist outside the window.

## Detect

`similar_tests.py` marks every `It`/`Specify` whose body (strings and numbers
normalized away) is at least 95% token-identical to another one in the same
file. The detail names the line of its near-twin; compare the two.

```tool
command: python3 .agents/tools/similar_tests.py --jsonl
violation_when: output_lines
output_format: jsonl
```

## Checks

- [redundant-case] Is a marked test redundant with a test already in the file — same entry point, equivalent inputs and same assertion, or a copy whose body differs only in literals or the function called and so belongs in an existing or new `DescribeTable` / table-driven loop?
- [mixed-scope] when=`(k8sClient|NewMock\w*\(|EXPECT\(\))` Does the change add a test that uses generated mocks (`NewMock…`, `EXPECT()`) instead of the API server to a `Describe`, table or suite whose other tests run against the real API server via `k8sClient` (or vice versa)?

## Examples

### Violation

```go source=projector/internal/infrastructure/cachekeys/cachekeys_test.go:171@ba54f7a6
		It("produces different keys for different teams", func() {
			_, lk1 := cachekeys.McpServer("/mcp/v1", "team-a")
			_, lk2 := cachekeys.McpServer("/mcp/v1", "team-b")
			Expect(lk1).NotTo(Equal(lk2))
		})
```

```go source=projector/internal/infrastructure/cachekeys/cachekeys_test.go:203@ba54f7a6
		It("produces different keys for different teams", func() {
			_, lk1 := cachekeys.AgentCard("/agent/v1", "team-a")
			_, lk2 := cachekeys.AgentCard("/agent/v1", "team-b")
			Expect(lk1).NotTo(Equal(lk2))
		})
```

```go synthetic="a unit case pushed into an envtest suite is rarely committed under a recognisable name; this is the scope mix being prevented"
var _ = Describe("Zone controller", func() {
	It("creates the gateway realm", func() {
		Expect(k8sClient.Create(ctx, zone)).To(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, realmKey, realm)).To(Succeed())
		}, timeout, interval).Should(Succeed())
	})

	It("returns an error when the janitor client fails", func() {
		mockK8s := fake.NewMockJanitorClient(GinkgoT())
		mockK8s.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).Return(errors.New("boom"))
		err := handler.CreateOrUpdate(cclient.WithClient(ctx, mockK8s), zone)
		Expect(err).To(HaveOccurred())
	})
})
```

### Allowed

```go source=identity/internal/handler/realm/handler_test.go:664@ba54f7a6
		It("should return an error when secret resolution fails", func() {
			realm := newValidRealm()
			realm.Status.AdminPassword = "$<secret-id>"

			overrideSecretsGet(func(_ context.Context, _ string) (string, error) {
				return "", fmt.Errorf("secret manager unavailable")
			})

			handler := NewHandlerRealm(unusedFactory)
			err := handler.Delete(ctx, realm)
```

```go source=projector/internal/infrastructure/idresolver_test.go:135@ba54f7a6
	for _, tc := range cases {
		Describe("Find"+capitalize(tc.entityType)+"ID", func() {
			It("should return cached ID on cache hit", func() {
				cache.Set(tc.entityType, tc.cachedKey, tc.cachedID)
				cache.Wait()
```
