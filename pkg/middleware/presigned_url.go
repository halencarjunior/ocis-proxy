package middleware

import (
	"context"
	"crypto/sha512"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/owncloud/ocis-pkg/v2/log"
	storepb "github.com/owncloud/ocis-store/pkg/proto/v0"
	"golang.org/x/crypto/pbkdf2"
)

// PresignedURL provides a middleware to check access secured by a presigned URL.
func PresignedURL(opts ...Option) func(next http.Handler) http.Handler {
	opt := newOptions(opts...)
	l := opt.Logger

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isSignedRequest(r) {
				if signedRequestIsValid(l, r, opt.Store) {
					ctx := r.Context()
					ctx = context.WithValue(ctx, ctxAccountIdQuery{}, r.URL.Query().Get("OC-Credential"))
					r = r.WithContext(ctx)

					next.ServeHTTP(w, r)
				} else {
					http.Error(w, "Invalid url signature", http.StatusUnauthorized)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func isSignedRequest(r *http.Request) bool {
	return r.URL.Query().Get("OC-Signature") != ""
}

func signedRequestIsValid(l log.Logger, r *http.Request, s storepb.StoreService) bool {
	// cheap checks first
	// TODO OC-Algorithm - defined the used algo (e.g. sha256 or sha512 - we should agree on one default algo and make this parameter optional)
	// OC-Credential - defines the user scope (shall we use the owncloud user id here - this might leak internal data ....) REQUIRED
	// OC-Date - defined the date the url was signed (ISO 8601 UTC) REQUIRED
	// OC-Expires - defines the expiry interval in seconds (between 1 and 604800 = 7 days) REQUIRED
	// TODO OC-Verb - defines for which http verb the request is valid - defaults to GET OPTIONAL
	// OC-Signature - the computed signature - server will verify the request upon this REQUIRED
	if r.URL.Query().Get("OC-Signature") == "" || r.URL.Query().Get("OC-Credential") == "" || r.URL.Query().Get("OC-Date") == "" || r.URL.Query().Get("OC-Expires") == "" || r.URL.Query().Get("OC-Verb") == "" {
		return false
	}

	if !strings.EqualFold(r.Method, r.URL.Query().Get("OC-Verb")) {
		return false
	}

	if t, err := time.Parse(time.RFC3339, r.URL.Query().Get("OC-Date")); err != nil {
		return false
	} else if expires, err := time.ParseDuration(r.URL.Query().Get("OC-Expires") + "s"); err != nil {
		return false
	} else {
		t.Add(expires)
		if t.After(time.Now()) {
			return false
		}
	}

	signingKey, err := getSigningKey(r.Context(), s, r.URL.Query().Get("OC-Credential"))
	if err != nil {
		l.Error().Err(err).Msg("could not retrieve signing key")
		return false
	}
	if len(signingKey) == 0 {
		l.Error().Err(err).Msg("signing key empty")
		return false
	}

	q := r.URL.Query()
	signature := q.Get("OC-Signature")
	q.Del("OC-Signature")
	r.URL.RawQuery = q.Encode()
	url := r.URL.String()
	if !r.URL.IsAbs() {
		url = "https://" + r.Host + url // TODO where do we get the scheme from
	}
	// the oc10 signature check: $hash = \hash_pbkdf2("sha512", $url, $signingKey, 10000, 64, false);
	// - sets the length of the output string to 64
	// - sets raw output to false ->  if raw_output is FALSE length corresponds to twice the byte-length of the derived key (as every byte of the key is returned as two hexits).
	// TODO change to length 128 in oc10?
	// fo golangs pbkdf2.Key we need to use 32 because it will be encoded into 64 hexits later
	hash := pbkdf2.Key([]byte(url), signingKey, 10000, 32, sha512.New)

	return hex.EncodeToString(hash) == signature
}

func getSigningKey(ctx context.Context, s storepb.StoreService, credential string) ([]byte, error) {
	res, err := s.Read(ctx, &storepb.ReadRequest{
		Options: &storepb.ReadOptions{
			Database: "proxy",
			Table:    "signing-keys",
		},
		Key: credential,
	})
	if err != nil || len(res.Records) < 1 {
		return []byte{}, err
	}

	return res.Records[0].Value, nil
}
