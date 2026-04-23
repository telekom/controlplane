// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package conjur_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cyberark/conjur-api-go/conjurapi/response"
	"github.com/stretchr/testify/mock"

	"github.com/telekom/controlplane/secret-manager/pkg/backend"
	"github.com/telekom/controlplane/secret-manager/pkg/backend/conjur"
	"github.com/telekom/controlplane/secret-manager/pkg/backend/conjur/bouncer"
	"github.com/telekom/controlplane/secret-manager/test/mocks"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var ErrNotFound = &response.ConjurError{
	Code:    404,
	Message: "Not Found",
}

var _ = Describe("Conjur Backend", func() {
	var writeAPI *mocks.MockConjurAPI
	var readAPI *mocks.MockConjurAPI

	BeforeEach(func() {
		writeAPI = mocks.NewMockConjurAPI(GinkgoT())
		readAPI = mocks.NewMockConjurAPI(GinkgoT())
	})

	Context("Parse ID", func() {
		It("should create a new Conjur backend", func() {
			conjurBackend := conjur.NewBackend(writeAPI, readAPI)
			Expect(conjurBackend).ToNot(BeNil())
		})

		It("should return an error on invalid secret id", func() {
			conjurBackend := conjur.NewBackend(writeAPI, readAPI)

			_, err := conjurBackend.ParseSecretId("my-secret-id")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("InvalidSecretId: invalid secret id 'my-secret-id'"))
		})

		It("should return a valid secret id", func() {
			conjurBackend := conjur.NewBackend(writeAPI, readAPI)

			rawSecretId := "test:my-team:my-app:clientSecret:checksum"
			secretId, err := conjurBackend.ParseSecretId(rawSecretId)
			Expect(err).ToNot(HaveOccurred())
			Expect(secretId).ToNot(BeNil())
			Expect(secretId.Env()).To(Equal("test"))
			Expect(secretId.VariableId()).To(Equal("controlplane/test/my-team/my-app/clientSecret"))
			Expect(secretId.String()).To(Equal("test:my-team:my-app:clientSecret:checksum"))
		})
	})

	Context("Get", func() {
		It("should return a secret with correct checksum", func() {
			ctx := context.Background()
			const value = "my-secret-value"

			correctCheckum := backend.MakeChecksum(value)
			conjurBackend := conjur.NewBackend(writeAPI, readAPI)

			readAPI.EXPECT().RetrieveSecret("controlplane/test/my-team/my-app/clientSecret").Return([]byte(value), nil).Times(1)

			secretId := conjur.New("test", "my-team", "my-app", "clientSecret", correctCheckum)

			res, err := conjurBackend.Get(ctx, secretId)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).ToNot(BeNil())
		})

		It("should fail with an invalid-checkum error", func() {
			ctx := context.Background()
			value := "my-secret-value"
			conjurBackend := conjur.NewBackend(writeAPI, readAPI)
			conjurBackend.MustMatchChecksum = true

			readAPI.EXPECT().RetrieveSecret("controlplane/test/my-team/my-app/clientSecret").Return([]byte(value), nil).Times(1)

			secretId := conjur.New("test", "my-team", "my-app", "clientSecret", "invalid-checksum")

			_, err := conjurBackend.Get(ctx, secretId)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("BadChecksum: bad checksum for secret test:my-team:my-app:clientSecret:invalid-checksum"))
		})

		It("should correct the checksum", func() {
			ctx := context.Background()
			value := "my-secret-value"
			conjurBackend := conjur.NewBackend(writeAPI, readAPI)

			readAPI.EXPECT().RetrieveSecret("controlplane/test/my-team/my-app/clientSecret").Return([]byte(value), nil).Times(1)

			secretId := conjur.New("test", "my-team", "my-app", "clientSecret", "invalid-checksum")

			secret, err := conjurBackend.Get(ctx, secretId)
			Expect(err).ToNot(HaveOccurred())
			Expect(secret.Id().String()).To(Equal("test:my-team:my-app:clientSecret:be22cbae9c15"))
		})

		It("should return an error if the conjur API fails", func() {
			ctx := context.Background()
			conjurBackend := conjur.NewBackend(writeAPI, readAPI)

			readAPI.EXPECT().RetrieveSecret("controlplane/test/my-team/my-app/clientSecret").Return(nil, fmt.Errorf("test-error")).Times(1)

			secretId := conjur.New("test", "my-team", "my-app", "clientSecret", "checksum")
			_, err := conjurBackend.Get(ctx, secretId)
			Expect(err).To(HaveOccurred())
		})

		It("should return a sub-secret", func() {
			ctx := context.Background()
			conjurBackend := conjur.NewBackend(writeAPI, readAPI)

			readAPI.EXPECT().RetrieveSecret("controlplane/test/my-team/my-app/externalSecrets").Return([]byte(`{"key1":"value1","key2/sub":"value2"}`), nil).Times(1)

			secretId := conjur.New("test", "my-team", "my-app", "externalSecrets/key1", "checksum")
			secretValue, err := conjurBackend.Get(ctx, secretId)
			Expect(err).ToNot(HaveOccurred())
			Expect(secretValue).ToNot(BeNil())
			Expect(secretValue.Value()).To(Equal("value1"))

			readAPI.EXPECT().RetrieveSecret("controlplane/test/my-team/my-app/externalSecrets").Return([]byte(`{"key1":"value1","key2/sub":"value2"}`), nil).Times(1)

			secretId = conjur.New("test", "my-team", "my-app", "externalSecrets/key2/sub", "checksum")
			secretValue, err = conjurBackend.Get(ctx, secretId)
			Expect(err).ToNot(HaveOccurred())
			Expect(secretValue).ToNot(BeNil())
			Expect(secretValue.Value()).To(Equal("value2"))
		})
	})

	Context("Set", func() {
		It("should not set a secret if it did not change", func() {
			ctx := context.Background()
			const value = "my-value"
			conjurBackend := conjur.NewBackend(writeAPI, readAPI)
			checksum := backend.MakeChecksum(value)

			readAPI.EXPECT().RetrieveSecret("controlplane/test/my-team/my-app/clientSecret").Return([]byte(value), nil).Times(1)

			secretId := conjur.New("test", "my-team", "my-app", "clientSecret", checksum)
			secretValue := backend.String(value)

			res, err := conjurBackend.Set(ctx, secretId, secretValue)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).ToNot(BeNil())
		})

		It("should not set a secret if it changed", func() {
			ctx := context.Background()
			const value = "my-value"
			conjurBackend := conjur.NewBackend(writeAPI, readAPI)
			checksum := backend.MakeChecksum(value)

			readAPI.EXPECT().RetrieveSecret("controlplane/test/my-team/my-app/clientSecret").Return([]byte(value), nil).Times(1)
			writeAPI.EXPECT().AddSecret("controlplane/test/my-team/my-app/clientSecret", "my-new-value").Return(nil).Times(1)

			secretId := conjur.New("test", "my-team", "my-app", "clientSecret", checksum)
			secretValue := backend.String("my-new-value")

			res, err := conjurBackend.Set(ctx, secretId, secretValue)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).ToNot(BeNil())
		})

		It("should create an initial secret if it does not exist", func() {
			ctx := context.Background()
			value := "my-value"
			conjurBackend := conjur.NewBackend(writeAPI, readAPI)

			readAPI.EXPECT().RetrieveSecret("controlplane/test/my-team/my-app/clientSecret").Return(nil, ErrNotFound).Times(1)
			writeAPI.EXPECT().AddSecret("controlplane/test/my-team/my-app/clientSecret", value).Return(nil).Times(1)

			secretId := conjur.New("test", "my-team", "my-app", "clientSecret", "")
			secretValue := backend.String(value)

			res, err := conjurBackend.Set(ctx, secretId, secretValue)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).ToNot(BeNil())
		})

		It("should not update the value of a secret if it not allowed", func() {
			ctx := context.Background()
			value := "my-value"
			checksum := backend.MakeChecksum(value)
			conjurBackend := conjur.NewBackend(writeAPI, readAPI)

			readAPI.EXPECT().RetrieveSecret("controlplane/test/my-team/my-app/clientSecret").Return([]byte(value), nil).Times(1)

			secretId := conjur.New("test", "my-team", "my-app", "clientSecret", checksum)
			secretValue := backend.InitialString("update-not-allowed-value")

			res, err := conjurBackend.Set(ctx, secretId, secretValue)
			Expect(err).NotTo(HaveOccurred())
			Expect(res).ToNot(BeNil())
			Expect(res.Value()).To(Equal(value))
		})

		Context("Strategy", func() {
			It("replace strategy should overwrite JSON value entirely", func() {
				ctx := context.Background()
				conjurBackend := conjur.NewBackend(writeAPI, readAPI)

				existing := `{"key1":"value1","key2":"value2"}`
				incoming := `{"key1":"updated"}`

				readAPI.EXPECT().RetrieveSecret("controlplane/test/my-team/my-app/externalSecrets").Return([]byte(existing), nil).Times(1)
				writeAPI.EXPECT().AddSecret("controlplane/test/my-team/my-app/externalSecrets", incoming).Return(nil).Times(1)

				secretId := conjur.New("test", "my-team", "my-app", "externalSecrets", "")
				secretValue := backend.String(incoming)

				res, err := conjurBackend.Set(ctx, secretId, secretValue, backend.WithWriteStrategy(backend.StrategyReplace))
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
			})

			It("merge strategy should shallow-merge JSON objects", func() {
				ctx := context.Background()
				conjurBackend := conjur.NewBackend(writeAPI, readAPI)

				existing := `{"key1":"value1","key2":"value2"}`
				incoming := `{"key1":"updated","key3":"new"}`

				readAPI.EXPECT().RetrieveSecret("controlplane/test/my-team/my-app/externalSecrets").Return([]byte(existing), nil).Times(1)
				// Merged result: key1=updated (overwritten), key2=value2 (preserved), key3=new (added)
				// json.Marshal produces keys in sorted order
				writeAPI.EXPECT().AddSecret("controlplane/test/my-team/my-app/externalSecrets", `{"key1":"updated","key2":"value2","key3":"new"}`).Return(nil).Times(1)

				secretId := conjur.New("test", "my-team", "my-app", "externalSecrets", "")
				secretValue := backend.String(incoming)

				res, err := conjurBackend.Set(ctx, secretId, secretValue, backend.WithWriteStrategy(backend.StrategyMerge))
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
			})

			It("merge strategy should preserve existing keys not in incoming JSON", func() {
				ctx := context.Background()
				conjurBackend := conjur.NewBackend(writeAPI, readAPI)

				existing := `{"secret1":"topsecret","secret2":"ineedcoffee"}`
				incoming := `{"secret1":"newsecret"}`

				readAPI.EXPECT().RetrieveSecret("controlplane/test/my-team/my-app/externalSecrets").Return([]byte(existing), nil).Times(1)
				// Merged: secret1=newsecret, secret2=ineedcoffee (preserved)
				// json.Marshal produces keys in sorted order
				writeAPI.EXPECT().AddSecret("controlplane/test/my-team/my-app/externalSecrets", `{"secret1":"newsecret","secret2":"ineedcoffee"}`).Return(nil).Times(1)

				secretId := conjur.New("test", "my-team", "my-app", "externalSecrets", "")
				secretValue := backend.String(incoming)

				res, err := conjurBackend.Set(ctx, secretId, secretValue, backend.WithWriteStrategy(backend.StrategyMerge))
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
			})

			It("merge strategy should overwrite plain strings (non-JSON)", func() {
				ctx := context.Background()
				conjurBackend := conjur.NewBackend(writeAPI, readAPI)

				existing := "old-secret"
				incoming := "new-secret"

				readAPI.EXPECT().RetrieveSecret("controlplane/test/my-team/my-app/clientSecret").Return([]byte(existing), nil).Times(1)
				writeAPI.EXPECT().AddSecret("controlplane/test/my-team/my-app/clientSecret", incoming).Return(nil).Times(1)

				secretId := conjur.New("test", "my-team", "my-app", "clientSecret", "")
				secretValue := backend.String(incoming)

				res, err := conjurBackend.Set(ctx, secretId, secretValue, backend.WithWriteStrategy(backend.StrategyMerge))
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
			})

			It("default (no strategy) should overwrite value directly", func() {
				ctx := context.Background()
				conjurBackend := conjur.NewBackend(writeAPI, readAPI)

				existing := `{"key1":"value1","key2":"value2"}`
				incoming := `{"key1":"updated"}`

				readAPI.EXPECT().RetrieveSecret("controlplane/test/my-team/my-app/externalSecrets").Return([]byte(existing), nil).Times(1)
				writeAPI.EXPECT().AddSecret("controlplane/test/my-team/my-app/externalSecrets", incoming).Return(nil).Times(1)

				secretId := conjur.New("test", "my-team", "my-app", "externalSecrets", "")
				secretValue := backend.String(incoming)

				// No strategy = replace (default)
				res, err := conjurBackend.Set(ctx, secretId, secretValue)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
			})

			It("merge strategy with identical JSON values should not write", func() {
				ctx := context.Background()
				conjurBackend := conjur.NewBackend(writeAPI, readAPI)

				existing := `{"key1":"value1"}`
				incoming := `{"key1":"value1"}`

				readAPI.EXPECT().RetrieveSecret("controlplane/test/my-team/my-app/externalSecrets").Return([]byte(existing), nil).Times(1)
				// No AddSecret call expected — value unchanged after merge

				secretId := conjur.New("test", "my-team", "my-app", "externalSecrets", "")
				secretValue := backend.String(incoming)

				res, err := conjurBackend.Set(ctx, secretId, secretValue, backend.WithWriteStrategy(backend.StrategyMerge))
				Expect(err).ToNot(HaveOccurred())
				Expect(res).ToNot(BeNil())
			})
		})
	})

	Context("Delete", func() {
		It("should delete a secret", func() {
			ctx := context.Background()
			conjurBackend := conjur.NewBackend(writeAPI, readAPI)

			writeAPI.EXPECT().AddSecret("controlplane/test/my-team/my-app/clientSecret", "").Return(nil).Times(1)

			secretId := conjur.New("test", "my-team", "my-app", "clientSecret", "checksum")
			err := conjurBackend.Delete(ctx, secretId)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("Concurrent Set with Bouncer", func() {
		It("should serialize concurrent Set calls on the same variable", func() {
			ctx := context.Background()
			const concurrency = 10
			const variableId = "controlplane/test/my-team/my-app/clientSecret"

			// Simulate a Conjur variable as an atomic value
			var storedValue atomic.Value
			storedValue.Store("initial-value")

			// Track the order of writes to verify serialization
			var writeOrder []string
			var writeOrderMu sync.Mutex

			// Create mocks that simulate real Conjur read-modify-write behavior
			// with timing delays to expose any race conditions
			writeAPIConcurrent := mocks.NewMockConjurAPI(GinkgoT())
			readAPIConcurrent := mocks.NewMockConjurAPI(GinkgoT())

			readAPIConcurrent.EXPECT().RetrieveSecret(variableId).RunAndReturn(
				func(id string) ([]byte, error) {
					// Small delay to widen the race window
					time.Sleep(time.Millisecond)
					return []byte(storedValue.Load().(string)), nil
				},
			)

			writeAPIConcurrent.EXPECT().AddSecret(variableId, mock.Anything).RunAndReturn(
				func(id, value string) error {
					// Small delay to widen the race window
					time.Sleep(time.Millisecond)
					storedValue.Store(value)
					writeOrderMu.Lock()
					writeOrder = append(writeOrder, value)
					writeOrderMu.Unlock()
					return nil
				},
			)

			// Create backend WITH bouncer
			locker := bouncer.NewLocker("test-secret-write")
			conjurBackend := conjur.NewBackend(writeAPIConcurrent, readAPIConcurrent).WithBouncer(locker)

			errs := make(chan error, concurrency)
			var wg sync.WaitGroup
			wg.Add(concurrency)

			for i := 0; i < concurrency; i++ {
				go func(idx int) {
					defer wg.Done()
					defer GinkgoRecover()
					secretId := conjur.New("test", "my-team", "my-app", "clientSecret", "")
					secretValue := backend.String(fmt.Sprintf("value-%d", idx))
					_, err := conjurBackend.Set(ctx, secretId, secretValue)
					errs <- err
				}(i)
			}

			wg.Wait()
			close(errs)

			for err := range errs {
				Expect(err).ToNot(HaveOccurred())
			}

			// All writes should have completed (serialized by bouncer)
			writeOrderMu.Lock()
			defer writeOrderMu.Unlock()
			Expect(writeOrder).To(HaveLen(concurrency))

			// The final stored value should match the last write
			finalValue := storedValue.Load().(string)
			Expect(finalValue).To(Equal(writeOrder[len(writeOrder)-1]))
		})
	})
})
