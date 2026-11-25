// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package accesstoken

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func signedToken(subject string, expiresIn time.Duration) string {
	GinkgoHelper()
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   subject,
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiresIn)),
	}).SignedString([]byte("test-key"))
	Expect(err).NotTo(HaveOccurred())
	return tok
}

func writeToken(path, tok string) {
	GinkgoHelper()
	Expect(os.WriteFile(path, []byte(tok), 0o600)).To(Succeed())
}

func expireCachedToken(at AccessToken) {
	fat := at.(*FileAccessToken)
	s := fat.current.Load()
	fat.current.Store(&tokenSnapshot{data: s.data, expiresAt: time.Now().Unix() - 1})
}

var _ = Describe("FileAccessToken", func() {
	var tokenPath string

	BeforeEach(func() {
		tokenPath = filepath.Join(GinkgoT().TempDir(), "token")
	})

	It("returns the same token to concurrent readers", func() {
		tok := signedToken("first", time.Hour)
		writeToken(tokenPath, tok)
		at := NewAccessToken(tokenPath)

		const readers = 32
		results := make([]string, readers)
		errs := make([]error, readers)
		var wg sync.WaitGroup
		for i := range readers {
			wg.Go(func() {
				results[i], errs[i] = at.Read()
			})
		}
		wg.Wait()

		for i := range readers {
			Expect(errs[i]).NotTo(HaveOccurred())
			Expect(results[i]).To(Equal(tok))
		}
	})

	It("re-reads the file once the cached token expires", func() {
		first := signedToken("first", time.Hour)
		writeToken(tokenPath, first)
		at := NewAccessToken(tokenPath)
		Expect(at.Read()).To(Equal(first))

		second := signedToken("second", time.Hour)
		writeToken(tokenPath, second)
		Expect(at.Read()).To(Equal(first), "a valid cached token must not trigger a file read")

		expireCachedToken(at)
		Expect(at.Read()).To(Equal(second))
	})

	It("keeps the cached token when a refresh fails", func() {
		first := signedToken("first", time.Hour)
		writeToken(tokenPath, first)
		at := NewAccessToken(tokenPath)
		Expect(at.Read()).To(Equal(first))
		expireCachedToken(at)

		writeToken(tokenPath, signedToken("expired", -time.Minute))
		_, err := at.Read()
		Expect(err).To(MatchError(ContainSubstring("token is expired")))

		writeToken(tokenPath, "not-a-jwt")
		_, err = at.Read()
		Expect(err).To(MatchError(ContainSubstring("failed to parse token")))

		fat := at.(*FileAccessToken)
		Expect(fat.current.Load().data).To(Equal(first))
		Expect(fat.IsExpired()).To(BeTrue())
	})
})
