// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package kubernetes_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/secret-manager/pkg/backend"
	"github.com/telekom/controlplane/secret-manager/pkg/backend/kubernetes"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func NewSecret(name, namespace string, data map[string]string) *corev1.Secret {
	secretData := make(map[string][]byte)
	for k, v := range data {
		secretData[k] = []byte(v)
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "secret-manager",
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: secretData,
	}
}

var _ = Describe("Kubernetes Backend", func() {

	var ctx context.Context
	var mockK8sClient client.Client

	BeforeEach(func() {
		ctx = context.Background()
		mockK8sClient = NewMockK8sClient()
	})

	Context("Parse ID", func() {
		It("should create a new Kubernetes backend", func() {
			k8sBackend := kubernetes.NewBackend(mockK8sClient)
			Expect(k8sBackend).ToNot(BeNil())
		})

		It("should return an error on invalid secret id", func() {
			k8sBackend := kubernetes.NewBackend(mockK8sClient)

			_, err := k8sBackend.ParseSecretId("my-secret-id")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("InvalidSecretId: invalid secret id 'my-secret-id'"))
		})

		It("should return a valid secret id", func() {
			k8sBackend := kubernetes.NewBackend(mockK8sClient)

			rawSecretId := "test:my-team:my-app:clientSecret:checksum"
			secretId, err := k8sBackend.ParseSecretId(rawSecretId)
			Expect(err).ToNot(HaveOccurred())
			Expect(secretId).ToNot(BeNil())
			Expect(secretId.Env()).To(Equal("test"))
			Expect(secretId.String()).To(Equal("test:my-team:my-app:clientSecret:checksum"))
		})
	})

	Context("Get Secret", func() {
		It("should return an error on invalid secret id", func() {
			k8sBackend := kubernetes.NewBackend(mockK8sClient)

			_, err := k8sBackend.Get(ctx, kubernetes.Id{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("IncorrectState: secrets \"secrets\" not found"))
		})

		It("should return a valid secret", func() {
			existingSecret := NewSecret("my-app", "poc--my-team", map[string]string{
				"clientSecret": "topsecret",
			})
			k8sBackend := kubernetes.NewBackend(NewMockK8sClient(existingSecret))

			secretId := kubernetes.New("poc", "my-team", "my-app", "clientSecret", "")

			secret, err := k8sBackend.Get(ctx, secretId)
			Expect(err).ToNot(HaveOccurred())
			Expect(secret).ToNot(BeNil())
		})

		It("should return an error when the resourceVersion does not match", func() {
			existingSecret := NewSecret("my-app", "poc--my-team", map[string]string{
				"clientSecret": "topsecret",
			})
			k8sBackend := kubernetes.NewBackend(NewMockK8sClient(existingSecret)).(*kubernetes.KubernetesBackend)
			k8sBackend.MatchResourceVersion = true

			secretId := kubernetes.New("poc", "my-team", "my-app", "clientSecret", "invalid-checksum")

			_, err := k8sBackend.Get(ctx, secretId)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("BadChecksum: bad checksum for secret poc:my-team:my-app:clientSecret:invalid-checksum"))
		})

		It("should return a team-secret", func() {
			existingSecret := NewSecret("my-team", "poc", map[string]string{
				"clientSecret": "topsecret",
				"teamToken":    "team-topsecret",
				"my-app":       `{"clientSecret": "topsecret"}`,
			})
			k8sBackend := kubernetes.NewBackend(NewMockK8sClient(existingSecret))

			secretId := kubernetes.New("poc", "my-team", "", "clientSecret", "")

			secret, err := k8sBackend.Get(ctx, secretId)
			Expect(err).ToNot(HaveOccurred())
			Expect(secret).ToNot(BeNil())
		})

		It("should return an error when the team-secret does not exist", func() {
			existingSecret := NewSecret("my-team", "poc", map[string]string{
				"clientSecret": "topsecret",
				"teamToken":    "team-topsecret",
				"my-app":       `{"clientSecret": "topsecret"}`,
			})
			k8sBackend := kubernetes.NewBackend(NewMockK8sClient(existingSecret))

			secretId := kubernetes.New("poc", "my-team", "", "invalid-secret", "")

			_, err := k8sBackend.Get(ctx, secretId)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("NotFound: resource poc:my-team::invalid-secret: not found"))
		})

		It("should return an error when the app-secret does not exist", func() {
			existingSecret := NewSecret("my-app", "poc--my-team", map[string]string{
				"clientSecret": "topsecret",
			})
			k8sBackend := kubernetes.NewBackend(NewMockK8sClient(existingSecret))

			secretId := kubernetes.New("poc", "my-team", "my-app", "invalid-secret", "")

			_, err := k8sBackend.Get(ctx, secretId)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("NotFound: resource poc:my-team:my-app:invalid-secret: not found"))
		})

		It("should return an error when the application does not exist", func() {
			existingSecret := NewSecret("my-app", "poc--my-team", map[string]string{
				"clientSecret": "topsecret",
			})
			k8sBackend := kubernetes.NewBackend(NewMockK8sClient(existingSecret))

			secretId := kubernetes.New("poc", "my-team", "invalid-app", "clientSecret", "")

			_, err := k8sBackend.Get(ctx, secretId)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("IncorrectState: secrets \"invalid-app\" not found"))
		})
	})

	Context("Get Secret canonical id", func() {
		var writes int

		newWriteCountingClient := func(objs ...client.Object) client.Client {
			writes = 0
			count := func() { writes++ }
			return fake.NewClientBuilder().WithObjects(objs...).WithInterceptorFuncs(interceptor.Funcs{
				Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
					count()
					return c.Create(ctx, obj, opts...)
				},
				Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
					count()
					return c.Update(ctx, obj, opts...)
				},
				Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
					count()
					return c.Patch(ctx, obj, patch, opts...)
				},
				Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
					count()
					return c.Delete(ctx, obj, opts...)
				},
			}).Build()
		}

		DescribeTable("nested secrets use the checksum of the extracted string value",
			func(inputChecksum string) {
				existingSecret := NewSecret("my-app", "poc--my-team", map[string]string{
					"externalSecrets": `{"foo":"synthetic-a","bar":{"x":1}}`,
				})
				k8sBackend := kubernetes.NewBackend(newWriteCountingClient(existingSecret))
				secretId := kubernetes.New("poc", "my-team", "my-app", "externalSecrets/foo", inputChecksum)
				originalString := secretId.String()

				secret, err := k8sBackend.Get(ctx, secretId)
				Expect(err).ToNot(HaveOccurred())
				Expect(secret.Value()).To(Equal("synthetic-a"))
				Expect(secret.Id().String()).To(Equal("poc:my-team:my-app:externalSecrets/foo:" + backend.MakeChecksum("synthetic-a")))
				Expect(secretId.String()).To(Equal(originalString))
				Expect(writes).To(BeZero())
			},
			Entry("hashless input", ""),
			Entry("stale checksum input", "stale"),
		)

		It("nested JSON object values hash the raw extracted string", func() {
			existingSecret := NewSecret("my-app", "poc--my-team", map[string]string{
				"externalSecrets": `{"bar":{"x":1}}`,
			})
			k8sBackend := kubernetes.NewBackend(newWriteCountingClient(existingSecret))
			secretId := kubernetes.New("poc", "my-team", "my-app", "externalSecrets/bar", "")

			secret, err := k8sBackend.Get(ctx, secretId)
			Expect(err).ToNot(HaveOccurred())
			Expect(secret.Value()).To(Equal(`{"x":1}`))
			Expect(secret.Id().String()).To(HaveSuffix(":" + backend.MakeChecksum(`{"x":1}`)))
		})

		It("nested checksum stays the same for unchanged and changes for changed values", func() {
			existingSecret := NewSecret("my-app", "poc--my-team", map[string]string{
				"externalSecrets": `{"foo":"synthetic-a"}`,
			})
			c := newWriteCountingClient(existingSecret)
			k8sBackend := kubernetes.NewBackend(c)
			secretId := kubernetes.New("poc", "my-team", "my-app", "externalSecrets/foo", "")

			first, err := k8sBackend.Get(ctx, secretId)
			Expect(err).ToNot(HaveOccurred())
			second, err := k8sBackend.Get(ctx, secretId)
			Expect(err).ToNot(HaveOccurred())
			Expect(second.Id().String()).To(Equal(first.Id().String()))
			Expect(writes).To(BeZero())

			obj := &corev1.Secret{}
			Expect(c.Get(ctx, secretId.ObjectKey(), obj)).To(Succeed())
			obj.Data["externalSecrets"] = []byte(`{"foo":"synthetic-b"}`)
			Expect(c.Update(ctx, obj)).To(Succeed())

			third, err := k8sBackend.Get(ctx, secretId)
			Expect(err).ToNot(HaveOccurred())
			Expect(third.Id().String()).ToNot(Equal(first.Id().String()))
			Expect(third.Id().String()).To(HaveSuffix(":" + backend.MakeChecksum("synthetic-b")))
		})

		It("top-level secrets use the object resourceVersion", func() {
			existingSecret := NewSecret("my-app", "poc--my-team", map[string]string{
				"clientSecret": "synthetic-a",
			})
			c := newWriteCountingClient(existingSecret)
			k8sBackend := kubernetes.NewBackend(c)
			secretId := kubernetes.New("poc", "my-team", "my-app", "clientSecret", "stale")

			secret, err := k8sBackend.Get(ctx, secretId)
			Expect(err).ToNot(HaveOccurred())

			obj := &corev1.Secret{}
			Expect(c.Get(ctx, secretId.ObjectKey(), obj)).To(Succeed())
			Expect(obj.GetResourceVersion()).ToNot(BeEmpty())
			Expect(secret.Id().String()).To(Equal("poc:my-team:my-app:clientSecret:" + obj.GetResourceVersion()))
			Expect(secretId.String()).To(Equal("poc:my-team:my-app:clientSecret:stale"))
			Expect(writes).To(BeZero())
		})

		It("keeps NotFound and IncorrectState classification", func() {
			existingSecret := NewSecret("my-app", "poc--my-team", map[string]string{
				"externalSecrets": `{"foo":"synthetic-a"}`,
			})
			k8sBackend := kubernetes.NewBackend(newWriteCountingClient(existingSecret))

			_, err := k8sBackend.Get(ctx, kubernetes.New("poc", "my-team", "my-app", "externalSecrets/missing", ""))
			Expect(backend.IsNotFoundErr(err)).To(BeTrue())

			_, err = k8sBackend.Get(ctx, kubernetes.New("poc", "my-team", "other-app", "clientSecret", ""))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(HavePrefix("IncorrectState:"))
		})
	})

	Context("Set Secret", func() {

		It("should return an error when the secret does not exist", func() {
			k8sBackend := kubernetes.NewBackend(mockK8sClient)

			secretId := kubernetes.New("poc", "my-team", "my-app", "clientSecret", "")
			secretValue := backend.String("topsecret")

			_, err := k8sBackend.Set(ctx, secretId, secretValue)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("IncorrectState: secrets \"my-app\" not found"))
		})

		It("should initially create a new secret", func() {
			existingSecret := NewSecret("my-team", "poc", map[string]string{
				"teamToken": "team-topsecret",
			})
			client := NewMockK8sClient(existingSecret)
			k8sBackend := kubernetes.NewBackend(client)

			secretId := kubernetes.New("poc", "my-team", "", "clientSecret", "")
			secretValue := backend.InitialString("topsecret")

			res, err := k8sBackend.Set(ctx, secretId, secretValue)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).ToNot(BeNil())
			Expect(res.Id().String()).To(Equal("poc:my-team::clientSecret:1000"))

			err = client.Get(ctx, secretId.ObjectKey(), existingSecret)
			Expect(err).NotTo(HaveOccurred())
			Expect(existingSecret.Data["clientSecret"]).To(Equal([]byte("topsecret")))
		})

		It("should update an existing app secret", func() {
			existingSecret := NewSecret("my-app", "poc--my-team", map[string]string{
				"clientSecret": "topsecret",
			})
			k8sBackend := kubernetes.NewBackend(NewMockK8sClient(existingSecret))

			secretId := kubernetes.New("poc", "my-team", "my-app", "clientSecret", "")
			secretValue := backend.String("new-topsecret")

			res, err := k8sBackend.Set(ctx, secretId, secretValue)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).ToNot(BeNil())
			Expect(res.Id().String()).To(Equal("poc:my-team:my-app:clientSecret:1000"))
		})

		It("should update an existing team secret", func() {
			existingSecret := NewSecret("my-team", "poc", map[string]string{
				"clientSecret": "topsecret",
				"teamToken":    "team-topsecret",
				"my-app":       `{"clientSecret": "topsecret"}`,
			})
			k8sBackend := kubernetes.NewBackend(NewMockK8sClient(existingSecret))

			secretId := kubernetes.New("poc", "my-team", "", "clientSecret", "")
			secretValue := backend.String("new-topsecret")

			res, err := k8sBackend.Set(ctx, secretId, secretValue)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).ToNot(BeNil())
			Expect(res.Id().String()).To(Equal("poc:my-team::clientSecret:1000"))
		})

		It("should not update an existing secret if its not allowed", func() {
			existingSecret := NewSecret("my-app", "poc--my-team", map[string]string{
				"clientSecret": "topsecret",
			})
			k8sBackend := kubernetes.NewBackend(NewMockK8sClient(existingSecret))

			secretId := kubernetes.New("poc", "my-team", "my-app", "clientSecret", "")
			secretValue := backend.InitialString("new-topsecret")

			res, err := k8sBackend.Set(ctx, secretId, secretValue)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).ToNot(BeNil())
			Expect(res.Id().String()).To(Equal("poc:my-team:my-app:clientSecret:"))
		})

		It("should not update an existing secret if the value has not been changed", func() {
			existingSecret := NewSecret("my-app", "poc--my-team", map[string]string{
				"clientSecret": "topsecret",
			})
			k8sBackend := kubernetes.NewBackend(NewMockK8sClient(existingSecret))

			secretId := kubernetes.New("poc", "my-team", "my-app", "clientSecret", "")
			secretValue := backend.String("topsecret")

			res, err := k8sBackend.Set(ctx, secretId, secretValue)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).ToNot(BeNil())
			Expect(res.Id().String()).To(Equal("poc:my-team:my-app:clientSecret:"))
		})
	})

	Context("Delete Secret", func() {

		It("should return an error when the secret does not exist", func() {
			k8sBackend := kubernetes.NewBackend(mockK8sClient)

			secretId := kubernetes.New("poc", "my-team", "my-app", "clientSecret", "")

			err := k8sBackend.Delete(ctx, secretId)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("NotFound: resource poc:my-team:my-app:clientSecret: not found"))
		})

		It("should delete an existing app secret", func() {
			existingSecret := NewSecret("my-app", "poc--my-team", map[string]string{
				"clientSecret":    "topsecret",
				"externalSecrets": "{}",
			})
			k8sClient := NewMockK8sClient(existingSecret)
			k8sBackend := kubernetes.NewBackend(k8sClient)

			secretId := kubernetes.New("poc", "my-team", "my-app", "clientSecret", "")

			err := k8sBackend.Delete(ctx, secretId)
			Expect(err).ToNot(HaveOccurred())

			// Verify that the secret was deleted
			err = k8sClient.Get(ctx, secretId.ObjectKey(), existingSecret)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(existingSecret.Data["externalSecrets"])).To(Equal("{}"))

		})

		It("should delete an existing team secret", func() {
			existingSecret := NewSecret("my-team", "poc", map[string]string{
				"clientSecret": "topsecret",
				"teamToken":    "team-topsecret",
				"my-app":       `{"clientSecret": "topsecret"}`,
			})
			k8sBackend := kubernetes.NewBackend(NewMockK8sClient(existingSecret))

			secretId := kubernetes.New("poc", "my-team", "", "clientSecret", "")

			err := k8sBackend.Delete(ctx, secretId)
			Expect(err).ToNot(HaveOccurred())
		})

	})
})
