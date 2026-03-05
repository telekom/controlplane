// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// MockConfigSource is a test implementation of ConfigSource
type MockConfigSource struct {
	data []byte
	err  error
}

func (m *MockConfigSource) Load(ctx context.Context) ([]byte, error) {
	return m.data, m.err
}

// customTestSource is a test implementation of ConfigSource
type customTestSource struct {
	value string
}

func (c *customTestSource) Load(ctx context.Context) ([]byte, error) {
	return []byte(c.value), nil
}

var _ = Describe("Loader", func() {
	ctx := context.Background()

	Describe("FileSource", func() {
		It("implements ConfigSource interface", func() {
			var _ ConfigSource = &FileSource{}
		})

		It("loads a valid YAML file", func() {
			source := &FileSource{Path: fixturePath(FixtureControllerDefaults)}
			data, err := source.Load(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(data).To(BeNil())
		})

		It("returns error for missing file", func() {
			source := &FileSource{Path: "/missing/file.yaml"}
			data, err := source.Load(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to read config file"))
			Expect(data).To(BeNil())
		})

		It("returns error for empty file", func() {
			source := &FileSource{Path: fixturePath(FixtureEmpty)}
			_, err := source.Load(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("config file is empty"))
		})

		It("accepts context parameter", func() {
			source := &FileSource{Path: fixturePath(FixtureControllerDefaults)}
			_, err := source.Load(ctx)
			Expect(err).ToNot(HaveOccurred())
		})

		It("accepts cancelled context", func() {
			source := &FileSource{Path: fixturePath(FixtureControllerDefaults)}
			cancelledCtx, cancel := context.WithCancel(ctx)
			cancel()
			_, err := source.Load(cancelledCtx)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("NewLoader", func() {
		It("creates a loader with the given source", func() {
			source := &FileSource{Path: fixturePath(FixtureControllerDefaults)}
			loader := NewLoader[EmptySpec](source)
			Expect(loader).NotTo(BeNil())
			Expect(loader.source).To(Equal(source))
		})
	})

	Describe("Loader.Load", func() {
		It("returns error for unsupported config source", func() {
			mockSource := &MockConfigSource{data: nil, err: fmt.Errorf("mock error")}
			loader := NewLoader[EmptySpec](mockSource)
			_, err := loader.Load(ctx, EmptySpecDefault)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("LoadFromFile convenience function", func() {
		It("returns error for non-existent file", func() {
			_, err := LoadFromFile[EmptySpec](ctx, "/nonexistent/config.yaml", EmptySpecDefault)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("ConfigSource Interface", func() {
		Describe("Mock ConfigSource for testing", func() {
			It("allows testing with custom data", func() {
				mockSource := &MockConfigSource{data: []byte("test"), err: nil}
				data, err := mockSource.Load(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(data).To(Equal([]byte("test")))
			})

			It("allows testing error scenarios", func() {
				mockSource := &MockConfigSource{data: nil, err: fmt.Errorf("test error")}
				data, err := mockSource.Load(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("test error"))
				Expect(data).To(BeNil())
			})
		})

		Describe("ConfigSource extensibility", func() {
			It("allows custom implementations", func() {
				customSource := &customTestSource{value: "custom-data"}
				data, err := customSource.Load(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(string(data)).To(Equal("custom-data"))
			})
		})
	})
})
