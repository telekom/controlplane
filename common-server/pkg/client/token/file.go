// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package accesstoken

import (
	"os"
	"sync/atomic"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/pkg/errors"
	"golang.org/x/sync/singleflight"
)

type AccessToken interface {
	Read() (string, error)
}

// FileAccessToken caches a JWT read from a file and re-reads the file once the
// cached token is within graceSeconds of its expiry. It is safe for concurrent use.
//
// If a refresh fails while the cached token has not reached its exp claim yet,
// the cached token is still returned, so a temporarily unreadable or stale file
// does not fail requests that the server would still accept.
type FileAccessToken struct {
	filePath     string
	graceSeconds int64
	jwtParser    *jwt.Parser
	readFile     func(name string) ([]byte, error)

	// current is replaced as a whole so readers never observe a torn token/expiry pair.
	current atomic.Pointer[tokenSnapshot]
	// refreshGroup shares one in-flight file read, including its error, among concurrent callers.
	refreshGroup singleflight.Group
}

type tokenSnapshot struct {
	data string
	// refreshAt is the token's exp claim minus the grace period, in Unix seconds.
	refreshAt int64
	// expiresAt is the token's exp claim, in Unix seconds.
	expiresAt int64
}

func (s *tokenSnapshot) needsRefresh(now int64) bool {
	return s == nil || now > s.refreshAt
}

func (s *tokenSnapshot) isUsable(now int64) bool {
	return s != nil && now <= s.expiresAt
}

func NewAccessToken(filePath string) AccessToken {
	return &FileAccessToken{
		filePath:     filePath,
		graceSeconds: 30,
		jwtParser:    jwt.NewParser(jwt.WithExpirationRequired()),
		readFile:     os.ReadFile,
	}
}

func (k *FileAccessToken) Read() (string, error) {
	if s := k.current.Load(); !s.needsRefresh(time.Now().Unix()) {
		return s.data, nil
	}
	return k.refresh()
}

func (k *FileAccessToken) refresh() (string, error) {
	v, err, _ := k.refreshGroup.Do("", func() (any, error) {
		// A refresh that finished after this caller's check may already have stored a fresh token.
		if s := k.current.Load(); !s.needsRefresh(time.Now().Unix()) {
			return s, nil
		}
		s, err := k.readTokenFile()
		if err != nil {
			return nil, err
		}
		k.current.Store(s)
		return s, nil
	})
	if err != nil {
		if prev := k.current.Load(); prev.isUsable(time.Now().Unix()) {
			return prev.data, nil
		}
		return "", err
	}
	return v.(*tokenSnapshot).data, nil
}

func (k *FileAccessToken) readTokenFile() (*tokenSnapshot, error) {
	if k.filePath == "" {
		return nil, errors.New("file path is empty")
	}
	data, err := k.readFile(k.filePath)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read token file %s", k.filePath)
	}
	raw := string(data)
	claims, err := k.parseJwt(raw)
	if err != nil {
		return nil, errors.Wrap(err, "failed to set expiresAt")
	}
	exp := claims.ExpiresAt.Unix()
	s := &tokenSnapshot{
		data:      raw,
		refreshAt: exp - k.graceSeconds,
		expiresAt: exp,
	}
	if s.needsRefresh(time.Now().Unix()) {
		return nil, errors.New("token is expired")
	}

	return s, nil
}

// IsExpired reports whether the cached token is missing or within the grace period of its expiry.
func (k *FileAccessToken) IsExpired() bool {
	return k.current.Load().needsRefresh(time.Now().Unix())
}

func (k *FileAccessToken) parseJwt(raw string) (*jwt.RegisteredClaims, error) {
	claims := &jwt.RegisteredClaims{}
	_, _, err := k.jwtParser.ParseUnverified(raw, claims)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse token")
	}
	return claims, nil
}
