// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package accesstoken

import (
	"os"
	"sync"
	"sync/atomic"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/pkg/errors"
)

type AccessToken interface {
	Read() (string, error)
}

// FileAccessToken caches a JWT read from a file and re-reads the file once the
// cached token is within graceSeconds of its expiry. It is safe for concurrent use.
type FileAccessToken struct {
	filePath     string
	graceSeconds int64
	jwtParser    *jwt.Parser

	// current is replaced as a whole so readers never observe a torn token/expiry pair.
	current atomic.Pointer[tokenSnapshot]
	// refreshMu ensures only one goroutine re-reads the file when the token expires.
	refreshMu sync.Mutex
}

type tokenSnapshot struct {
	data string
	// expiresAt is the token's exp claim minus the grace period, in Unix seconds.
	expiresAt int64
}

func (s *tokenSnapshot) isExpired(now int64) bool {
	return s == nil || now > s.expiresAt
}

func NewAccessToken(filePath string) AccessToken {
	return &FileAccessToken{
		filePath:     filePath,
		graceSeconds: 30,
		jwtParser:    jwt.NewParser(jwt.WithExpirationRequired()),
	}
}

func (k *FileAccessToken) Read() (string, error) {
	if s := k.current.Load(); !s.isExpired(time.Now().Unix()) {
		return s.data, nil
	}
	return k.refresh()
}

func (k *FileAccessToken) refresh() (string, error) {
	k.refreshMu.Lock()
	defer k.refreshMu.Unlock()

	// Another goroutine may have refreshed the token while this one waited for the lock.
	if s := k.current.Load(); !s.isExpired(time.Now().Unix()) {
		return s.data, nil
	}

	s, err := k.readTokenFile()
	if err != nil {
		return "", err
	}
	k.current.Store(s)
	return s.data, nil
}

func (k *FileAccessToken) readTokenFile() (*tokenSnapshot, error) {
	if k.filePath == "" {
		return nil, errors.New("file path is empty")
	}
	data, err := os.ReadFile(k.filePath)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read token file %s", k.filePath)
	}
	raw := string(data)
	claims, err := k.parseJwt(raw)
	if err != nil {
		return nil, errors.Wrap(err, "failed to set expiresAt")
	}
	s := &tokenSnapshot{
		data:      raw,
		expiresAt: claims.ExpiresAt.Unix() - k.graceSeconds,
	}
	if s.isExpired(time.Now().Unix()) {
		return nil, errors.New("token is expired")
	}

	return s, nil
}

// IsExpired checks if the token is expired
func (k *FileAccessToken) IsExpired() bool {
	return k.current.Load().isExpired(time.Now().Unix())
}

func (k *FileAccessToken) parseJwt(raw string) (*jwt.RegisteredClaims, error) {
	claims := &jwt.RegisteredClaims{}
	_, _, err := k.jwtParser.ParseUnverified(raw, claims)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse token")
	}
	return claims, nil
}
