// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/mock"
	"github.com/telekom/controlplane/rover-ctl/pkg/config"
	"github.com/telekom/controlplane/rover-ctl/pkg/handlers"
	"github.com/telekom/controlplane/rover-ctl/pkg/types"
	"github.com/telekom/controlplane/rover-ctl/test/mocks"
)

var _ = Describe("NormalizeLegacyArgs", func() {
	BeforeEach(func() {
		viper.Reset()
		config.Initialize()
		DeferCleanup(func() {
			viper.Reset()
			config.Initialize()
		})
	})

	DescribeTable("rewrites only the legacy -name flag of resource commands",
		func(args, expected []string) {
			Expect(NormalizeLegacyArgs(NewRootCommand(), args)).To(Equal(expected))
		},
		Entry("legacy -name=<value>",
			[]string{"resource", "--kind", "Rover", "--get", "-name=my-app"},
			[]string{"resource", "--kind", "Rover", "--get", "--name=my-app"}),
		Entry("legacy -name <value>",
			[]string{"resource", "--kind", "Rover", "--delete", "-name", "my-app"},
			[]string{"resource", "--kind", "Rover", "--delete", "--name", "my-app"}),
		Entry("subcommand with -name=<value>",
			[]string{"resource", "get", "--kind", "Rover", "-name=my-app"},
			[]string{"resource", "get", "--kind", "Rover", "--name=my-app"}),
		Entry("root flags before the command",
			[]string{"--format", "yaml", "resource", "delete", "--kind", "Rover", "-name=my-app"},
			[]string{"--format", "yaml", "resource", "delete", "--kind", "Rover", "--name=my-app"}),
		Entry("value named like a subcommand",
			[]string{"resource", "--kind", "Rover", "--delete", "-name", "get"},
			[]string{"resource", "--kind", "Rover", "--delete", "--name", "get"}),
		Entry("-name as a flag value is kept",
			[]string{"resource", "--kind", "-name", "--list"},
			[]string{"resource", "--kind", "-name", "--list"}),
		Entry("-name as a shorthand flag value is kept",
			[]string{"resource", "get", "--kind", "Rover", "-n", "-name"},
			[]string{"resource", "get", "--kind", "Rover", "-n", "-name"}),
		Entry("-name as a combined shorthand flag value is kept",
			[]string{"resource", "get", "--kind", "Rover", "-dn", "-name"},
			[]string{"resource", "get", "--kind", "Rover", "-dn", "-name"}),
		Entry("-name after the terminator is kept",
			[]string{"resource", "--kind", "Rover", "--list", "--", "-name=my-app"},
			[]string{"resource", "--kind", "Rover", "--list", "--", "-name=my-app"}),
		Entry("modern forms are kept",
			[]string{"resource", "get", "--kind", "Rover", "--name", "my-app"},
			[]string{"resource", "get", "--kind", "Rover", "--name", "my-app"}),
		Entry("other commands are kept",
			[]string{"get-info", "-name=my-app"},
			[]string{"get-info", "-name=my-app"}),
		Entry("similar flags are kept",
			[]string{"resource", "--kind", "Rover", "--get", "-names=my-app"},
			[]string{"resource", "--kind", "Rover", "--get", "-names=my-app"}),
		Entry("no arguments", []string{}, []string{}),
	)

	It("does not modify the input slice", func() {
		args := []string{"resource", "--kind", "Rover", "--get", "-name=my-app"}
		NormalizeLegacyArgs(NewRootCommand(), args)
		Expect(args[4]).To(Equal("-name=my-app"))
	})
})

var _ = Describe("Root command legacy resource syntax", func() {
	var (
		mockHandler *mocks.MockResourceHandler
		stdout      *bytes.Buffer
		stderr      *bytes.Buffer
		obj         *types.UnstructuredObject
	)

	BeforeEach(func() {
		viper.Reset()
		GinkgoT().Setenv("ROVER_SERVER_URL", "")
		GinkgoT().Setenv("ROVER_TOKEN_URL", "")
		GinkgoT().Setenv("ROVER_TOKEN", tokenWith(func(map[string]any) {}))
		config.Initialize()
		DeferCleanup(func() {
			viper.Reset()
			config.Initialize()
			handlers.ResetRegistryForTest()
		})

		mockHandler = mocks.NewMockResourceHandler(GinkgoT())
		mockHandler.EXPECT().Priority().Return(100).Maybe()
		handlers.RegisterHandler("TestKind", "tcp.ei.telekom.de/v1", mockHandler)
		stdout = &bytes.Buffer{}
		stderr = &bytes.Buffer{}
		obj = &types.UnstructuredObject{Content: map[string]any{
			"apiVersion": "tcp.ei.telekom.de/v1",
			"kind":       "TestKind",
			"metadata":   map[string]any{"name": "my-app"},
		}}
	})

	execute := func(args ...string) error {
		root := NewRootCommand()
		root.SetOut(stdout)
		root.SetErr(stderr)
		root.SetArgs(NormalizeLegacyArgs(root, args))
		return root.Execute()
	}

	It("gets a resource with -name=<resourceName>", func() {
		mockHandler.EXPECT().Get(mock.Anything, "my-app").Return(obj, nil).Once()
		Expect(execute("resource", "--kind", "TestKind", "--get", "-name=my-app")).To(Succeed())
		Expect(stdout.String()).To(ContainSubstring("my-app"))
	})

	It("lists resources with the legacy --list selector", func() {
		mockHandler.EXPECT().List(mock.Anything).Return([]any{obj}, nil).Once()
		Expect(execute("resource", "--kind", "TestKind", "--list")).To(Succeed())
		Expect(stdout.String()).To(ContainSubstring("my-app"))
		Expect(stdout.String()).NotTo(ContainSubstring("deprecated"))
		Expect(stderr.String()).To(ContainSubstring(`use "roverctl resource list" instead`))
	})

	It("does not print a deprecation notice for the list subcommand", func() {
		mockHandler.EXPECT().List(mock.Anything).Return([]any{obj}, nil).Once()
		Expect(execute("resource", "list", "--kind", "TestKind")).To(Succeed())
		Expect(stdout.String()).To(ContainSubstring("my-app"))
		Expect(stderr.String()).NotTo(ContainSubstring("deprecated"))
	})

	It("rejects -name without a value instead of deleting anything", func() {
		err := execute("resource", "--kind", "TestKind", "--delete", "-name=")
		Expect(err).To(MatchError(ContainSubstring("--name must not be empty")))
	})
})

var _ = Describe("Resource delete against production handlers", func() {
	type request struct {
		method, path string
		body         string
	}

	var (
		server       *httptest.Server
		mu           sync.Mutex
		requests     []request
		deleteStatus int
		statusStatus int
	)

	recorded := func() []request {
		mu.Lock()
		defer mu.Unlock()
		return append([]request(nil), requests...)
	}

	BeforeEach(func() {
		requests = nil
		deleteStatus = http.StatusNoContent
		statusStatus = http.StatusNotFound
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			mu.Lock()
			requests = append(requests, request{method: r.Method, path: r.URL.Path, body: string(body)})
			mu.Unlock()
			if r.Method == http.MethodDelete {
				w.WriteHeader(deleteStatus)
				return
			}
			w.WriteHeader(statusStatus)
		}))
		DeferCleanup(server.Close)

		viper.Reset()
		GinkgoT().Setenv("ROVER_SERVER_URL", "")
		GinkgoT().Setenv("ROVER_TOKEN_URL", "")
		GinkgoT().Setenv("ROVER_TOKEN", tokenWith(func(f map[string]any) { f["server_url"] = server.URL }))
		config.Initialize()
		viper.Set("access.token", "local-test-token")
		viper.Set("poll.interval", "10ms")
		viper.Set("timeout.status", "2s")
		DeferCleanup(func() {
			viper.Reset()
			config.Initialize()
			handlers.ResetRegistryForTest()
		})
	})

	execute := func(args ...string) error {
		root := NewRootCommand()
		root.SetOut(&bytes.Buffer{})
		root.SetErr(&bytes.Buffer{})
		root.SetArgs(NormalizeLegacyArgs(root, args))
		return root.Execute()
	}

	forms := func(kind string) map[string][]string {
		return map[string][]string{
			"modern":     {"resource", "delete", "--kind", kind, "--name", "my-app"},
			"deprecated": {"resource", "--kind", kind, "--delete", "-name=my-app"},
		}
	}

	DescribeTable("sends a bodyless DELETE and waits until the resource is gone",
		func(kind, resource string) {
			for form, args := range forms(kind) {
				mu.Lock()
				requests = nil
				mu.Unlock()

				Expect(execute(args...)).To(Succeed(), form)

				reqs := recorded()
				Expect(reqs).NotTo(BeEmpty(), form)
				Expect(reqs[0]).To(Equal(request{method: http.MethodDelete, path: "/rover/api/" + resource + "/my-group--my-team--my-app"}), form)
				Expect(reqs[1:]).NotTo(BeEmpty(), form)
				for _, r := range reqs[1:] {
					Expect(r.method).To(Equal(http.MethodGet), form)
					Expect(r.path).To(Equal("/rover/api/"+resource+"/my-group--my-team--my-app/status"), form)
				}
			}
		},
		Entry("Rover", "Rover", "rovers"),
		Entry("ApiSpecification", "ApiSpecification", "apispecifications"),
		Entry("EventSpecification", "EventSpecification", "eventspecifications"),
		Entry("FileSpecification", "FileSpecification", "filespecifications"),
		Entry("ApiRoadmap", "ApiRoadmap", "apiroadmaps"),
		Entry("ApiChangelog", "ApiChangelog", "apichangelogs"),
	)

	DescribeTable("returns a rejected DELETE as error without polling",
		func(form string) {
			deleteStatus = http.StatusForbidden
			err := execute(forms("Rover")[form]...)
			Expect(err).To(MatchError(ContainSubstring(`failed to delete Rover "my-app"`)))
			Expect(recorded()).To(HaveLen(1))
		},
		Entry("modern", "modern"),
		Entry("deprecated", "deprecated"),
	)

	DescribeTable("returns a network failure as error without polling",
		func(form string) {
			server.Close()
			err := execute(forms("Rover")[form]...)
			Expect(err).To(MatchError(ContainSubstring(`failed to delete Rover "my-app"`)))
			Expect(recorded()).To(BeEmpty())
		},
		Entry("modern", "modern"),
		Entry("deprecated", "deprecated"),
	)

	DescribeTable("returns a failed wait as error",
		func(form string) {
			statusStatus = http.StatusInternalServerError
			err := execute(forms("Rover")[form]...)
			Expect(err).To(MatchError(ContainSubstring(`failed to wait for Rover "my-app" to be deleted`)))
			Expect(recorded()[0].method).To(Equal(http.MethodDelete))
		},
		Entry("modern", "modern"),
		Entry("deprecated", "deprecated"),
	)

	It("keeps file based delete sending the resource definition", func() {
		file := GinkgoT().TempDir() + "/rover.yaml"
		Expect(writeFile(file, "apiVersion: tcp.ei.telekom.de/v1\nkind: Rover\nmetadata:\n  name: my-app\nspec:\n  zone: test\n")).To(Succeed())

		Expect(execute("delete", "--file", file)).To(Succeed())

		reqs := recorded()
		Expect(reqs[0].method).To(Equal(http.MethodDelete))
		Expect(reqs[0].body).To(ContainSubstring(`"zone":"test"`))
	})
})

func writeFile(path, content string) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating %s: %w", path, err)
	}
	defer f.Close()
	_, err = strings.NewReader(content).WriteTo(f)
	return err
}
