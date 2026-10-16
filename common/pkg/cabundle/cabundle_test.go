// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package cabundle_test

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"time"

	"github.com/telekom/controlplane/common/pkg/cabundle"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func certCount(data []byte) int {
	n := 0
	for {
		var block *pem.Block
		block, data = pem.Decode(data)
		if block == nil {
			return n
		}
		n++
	}
}

var _ = Describe("Canonicalize", func() {
	It("returns nil without input", func() {
		out, err := cabundle.Canonicalize()
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(BeNil())
	})

	It("accepts CA certificates surrounded by whitespace", func() {
		ca := newCA("a")
		out, err := cabundle.Canonicalize(append(append([]byte("\n  \t"), ca...), []byte("\n\n")...))
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(Equal(ca))
	})

	It("accepts expired CA certificates", func() {
		ca := newCertPEM(certOpts{isCA: true, notAfter: time.Now().Add(-24 * time.Hour), commonName: "expired"})
		_, err := cabundle.Canonicalize(ca)
		Expect(err).NotTo(HaveOccurred())
	})

	It("accepts CA certificates without key usage", func() {
		_, err := cabundle.Canonicalize(newCertPEM(certOpts{isCA: true, commonName: "nokeyusage"}))
		Expect(err).NotTo(HaveOccurred())
	})

	It("deduplicates and orders deterministically", func() {
		a, b, c := newCA("a"), newCA("b"), newCA("c")
		out1, err := cabundle.Canonicalize(bytes.Join([][]byte{a, b}, nil), bytes.Join([][]byte{c, a}, nil))
		Expect(err).NotTo(HaveOccurred())
		out2, err := cabundle.Canonicalize(bytes.Join([][]byte{c, b, a, b}, []byte("\n")))
		Expect(err).NotTo(HaveOccurred())
		Expect(out1).To(Equal(out2))
		Expect(certCount(out1)).To(Equal(3))
	})

	DescribeTable("rejects invalid input",
		func(input func() []byte) {
			_, err := cabundle.Canonicalize(input())
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, cabundle.ErrInvalidBundle)).To(BeTrue(), err.Error())
			var vErr *cabundle.ValidationError
			Expect(errors.As(err, &vErr)).To(BeTrue())
		},
		Entry("empty", func() []byte { return nil }),
		Entry("whitespace only", func() []byte { return []byte(" \n\t") }),
		Entry("leading junk", func() []byte { return append([]byte("junk\n"), newCA("a")...) }),
		Entry("trailing junk", func() []byte { return append(newCA("a"), []byte("junk")...) }),
		Entry("malformed block", func() []byte {
			return []byte("-----BEGIN CERTIFICATE-----\n!!!!\n-----END CERTIFICATE-----\n")
		}),
		Entry("unparseable certificate", func() []byte {
			return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("nope")})
		}),
		Entry("private key", func() []byte {
			return append(newCA("a"), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("k")})...)
		}),
		Entry("pem headers", func() []byte {
			block, _ := pem.Decode(newCA("a"))
			block.Headers = map[string]string{"Proc-Type": "4,ENCRYPTED"}
			return pem.EncodeToMemory(block)
		}),
		Entry("leaf certificate", func() []byte { return newCertPEM(certOpts{commonName: "leaf"}) }),
		Entry("IsCA without valid basic constraints", func() []byte {
			return newCertPEM(certOpts{isCA: false, noBasic: true, commonName: "nobasic"})
		}),
		Entry("CA without cert sign usage", func() []byte {
			return newCertPEM(certOpts{isCA: true, keyUsage: x509.KeyUsageDigitalSignature, commonName: "nosign"})
		}),
	)

	It("rejects any invalid set in the union", func() {
		_, err := cabundle.Canonicalize(newCA("a"), []byte("junk"))
		Expect(err).To(MatchError(cabundle.ErrInvalidBundle))
	})

	It("enforces the managed ConfigMap size limit", func() {
		Expect(cabundle.MaxBundleBytes).To(Equal(1024*1024 - len(cabundle.ManagedBundleKey)))
		var sets [][]byte
		size := 0
		for size <= cabundle.MaxBundleBytes {
			c := newCA("x")
			sets = append(sets, c)
			size += len(c)
		}
		_, err := cabundle.Canonicalize(sets...)
		Expect(err).To(MatchError(ContainSubstring("exceeding limit")))
		Expect(errors.Is(err, cabundle.ErrInvalidBundle)).To(BeTrue())
	})
})
