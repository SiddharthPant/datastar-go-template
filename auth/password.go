package auth

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"datastar-go/config"

	"golang.org/x/crypto/argon2"
)

var (
	// ErrInvalidHash in returned by ComparePasswordAndHash if the provided
	// hash isn't in the expected format.
	ErrInvalidHash = errors.New("argon2id: hash is not in the correct format")

	// ErrIncompatibleVariant is returned by ComparePasswordAndHash if the
	// provided hash was created using a unsupported variant of Argon2.
	// Currently only argon2id is supported by this package.
	ErrIncompatibleVariant = errors.New("argon2id: incompatible variant of argon2")

	// ErrIncompatibleVersion is returned by ComparePasswordAndHash if the
	// provided hash was created using a different version of Argon2.
	ErrIncompatibleVersion = errors.New("argon2id: incompatible version of argon2")
)

const (
	argonMemoryKiB   uint32 = 64 * 1024
	argonIterations  uint32 = 3
	argonParallelism uint8  = 4
	argonSaltLength  uint32 = 16
	argonKeyLength   uint32 = 32
)

func Validate(password string) error {
	minPasswordLength := config.Env.MinPasswordLength
	if utf8.RuneCountInString(password) < minPasswordLength {
		return fmt.Errorf("password must be at least %d characters", minPasswordLength)
	}
	return nil
}

func GenerateHash[T interface{ string | []byte }](password T) (string, error) {
	salt, err := generateRandomBytes(argonSaltLength)
	if err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, argonIterations, argonMemoryKiB, argonParallelism, argonKeyLength)
	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemoryKiB, argonIterations, argonParallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

type HashParams struct {
	Version     int
	MemoryKiB   uint32
	Iterations  uint32
	Parallelism uint8
}

func DecodeHash(hash string) (p *HashParams, salt, key []byte, err error) {
	r := strings.NewReader(hash)

	_, err = fmt.Fscanf(r, "$argon2id$")
	if err != nil {
		return nil, nil, nil, ErrIncompatibleVariant
	}

	var version int
	_, err = fmt.Fscanf(r, "v=%d$", &version)
	if err != nil {
		return nil, nil, nil, err
	}
	if version != argon2.Version {
		return nil, nil, nil, ErrIncompatibleVersion
	}

	p = &HashParams{}
	_, err = fmt.Fscanf(r, "m=%d,t=%d,p=%d$", &p.MemoryKiB, &p.Iterations, &p.Parallelism)
	if err != nil {
		return nil, nil, nil, err
	}

	rest, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, nil, err
	}
	if bytes.ContainsAny(rest, "\r\n") { // base64 decoder ignores these
		return nil, nil, nil, ErrInvalidHash
	}

	var i int
	if i = bytes.IndexByte(rest, '$'); i == -1 {
		return nil, nil, nil ErrInvalidHash
	}

	b64Enc := base64.RawStdEncoding.Strict()

	salt = make([]byte, b64Enc.DecodedLen(i))
	_, err = b64Enc.Decode(salt, rest[:i])
	if err != nil
}

func generateRandomBytes(n uint32) ([]byte, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	if err != nil {
		return nil, err
	}

	return b, nil
}
