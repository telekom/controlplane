// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package accesstoken

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
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

// setCachedToken replaces the cached token's refresh and expiry times, relative to now in seconds.
func setCachedToken(at *FileAccessToken, refreshOffset, expiryOffset int64) {
	now := time.Now().Unix()
	s := at.current.Load()
	at.current.Store(&tokenSnapshot{data: s.data, refreshAt: now + refreshOffset, expiresAt: now + expiryOffset})
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

	It("re-reads the file once the cached token needs a refresh", func() {
		first := signedToken("first", time.Hour)
		writeToken(tokenPath, first)
		at := NewAccessToken(tokenPath).(*FileAccessToken)
		Expect(at.Read()).To(Equal(first))

		second := signedToken("second", time.Hour)
		writeToken(tokenPath, second)
		Expect(at.Read()).To(Equal(first), "a valid cached token must not trigger a file read")

		setCachedToken(at, -1, 10)
		Expect(at.Read()).To(Equal(second))
	})

	It("shares one failed refresh among concurrent callers and retries on a later call", func() {
		writeToken(tokenPath, "not-a-jwt")
		at := NewAccessToken(tokenPath).(*FileAccessToken)

		var reads atomic.Int32
		entered := make(chan struct{})
		release := make(chan struct{})
		at.readFile = func(name string) ([]byte, error) {
			if reads.Add(1) == 1 {
				close(entered)
				<-release
			}
			return os.ReadFile(name)
		}

		const callers = 32
		errs := make([]error, callers)
		var wg sync.WaitGroup
		wg.Go(func() { _, errs[0] = at.Read() })
		Eventually(entered).WithTimeout(time.Second).WithPolling(time.Millisecond).Should(BeClosed())

		var started atomic.Int32
		for i := 1; i < callers; i++ {
			wg.Go(func() {
				started.Add(1)
				_, errs[i] = at.Read()
			})
		}
		Eventually(started.Load).WithTimeout(time.Second).WithPolling(time.Millisecond).Should(BeEquivalentTo(callers - 1))
		// Give the started callers time to join the in-flight refresh before it completes.
		time.Sleep(50 * time.Millisecond)
		close(release)
		wg.Wait()

		Expect(reads.Load()).To(BeEquivalentTo(1))
		for _, err := range errs {
			Expect(err).To(MatchError(ContainSubstring("failed to parse token")))
		}

		fixed := signedToken("fixed", time.Hour)
		writeToken(tokenPath, fixed)
		Expect(at.Read()).To(Equal(fixed))
		Expect(reads.Load()).To(BeEquivalentTo(2))
	})

	It("serves the cached token until its exp claim when a refresh fails", func() {
		first := signedToken("first", time.Hour)
		writeToken(tokenPath, first)
		at := NewAccessToken(tokenPath).(*FileAccessToken)
		Expect(at.Read()).To(Equal(first))

		writeToken(tokenPath, "not-a-jwt")
		setCachedToken(at, -1, 10)
		Expect(at.Read()).To(Equal(first))
		Expect(at.IsExpired()).To(BeTrue())

		setCachedToken(at, -20, -1)
		_, err := at.Read()
		Expect(err).To(MatchError(ContainSubstring("failed to parse token")))

		writeToken(tokenPath, signedToken("expired", -time.Minute))
		_, err = at.Read()
		Expect(err).To(MatchError(ContainSubstring("token is expired")))
		Expect(at.current.Load().data).To(Equal(first), "a failed refresh must not replace the cached token")
	})
})
