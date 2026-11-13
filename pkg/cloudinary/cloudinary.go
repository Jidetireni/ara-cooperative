// Package cloudinary signs direct browser uploads to Cloudinary and verifies
// what Cloudinary returns. The API secret stays on the server: the server
// chooses the public ID and signs it, the browser sends the file straight to
// Cloudinary, and clients build delivery URLs from the stored public ID.
package cloudinary

import (
	"crypto/sha1"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ErrNotConfigured is returned when the cloud name or credentials are missing.
var ErrNotConfigured = errors.New("cloudinary is not configured")

// AllowedFormats limits signed uploads to photographs.
const AllowedFormats = "jpg,png"

type Client struct {
	cloudName string
	apiKey    string
	apiSecret string
	now       func() time.Time
}

func New(cloudName, apiKey, apiSecret string) *Client {
	return &Client{cloudName: cloudName, apiKey: apiKey, apiSecret: apiSecret, now: time.Now}
}

// Enabled reports whether uploads can be signed.
func (c *Client) Enabled() bool {
	return c != nil && c.cloudName != "" && c.apiKey != "" && c.apiSecret != ""
}

// UploadSignature is everything the browser sends to
// https://api.cloudinary.com/v1_1/<cloud_name>/image/upload besides the file.
// Cloudinary rejects the upload if any signed field is changed, and the
// signature expires one hour after Timestamp.
type UploadSignature struct {
	CloudName      string `json:"cloud_name"`
	APIKey         string `json:"api_key"`
	Timestamp      int64  `json:"timestamp"`
	PublicID       string `json:"public_id"`
	Overwrite      bool   `json:"overwrite"`
	Invalidate     bool   `json:"invalidate"`
	AllowedFormats string `json:"allowed_formats"`
	Signature      string `json:"signature"`
}

// Sign returns Cloudinary's request signature: SHA-1 of the sorted
// key=value parameters joined by "&", followed by the API secret.
func Sign(params map[string]string, apiSecret string) string {
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+params[key])
	}
	sum := sha1.Sum([]byte(strings.Join(parts, "&") + apiSecret))
	return hex.EncodeToString(sum[:])
}

// SignUpload authorises one upload to publicID, replacing any image there.
func (c *Client) SignUpload(publicID string) (*UploadSignature, error) {
	if !c.Enabled() {
		return nil, ErrNotConfigured
	}
	timestamp := c.now().Unix()
	params := map[string]string{
		"allowed_formats": AllowedFormats,
		"invalidate":      "true",
		"overwrite":       "true",
		"public_id":       publicID,
		"timestamp":       strconv.FormatInt(timestamp, 10),
	}
	return &UploadSignature{
		CloudName: c.cloudName, APIKey: c.apiKey, Timestamp: timestamp, PublicID: publicID,
		Overwrite: true, Invalidate: true, AllowedFormats: AllowedFormats, Signature: Sign(params, c.apiSecret),
	}, nil
}

// VerifyUpload checks the signature in Cloudinary's upload response, which
// signs public_id and version with the API secret. It proves the image was
// really stored under publicID at that version.
func (c *Client) VerifyUpload(publicID string, version int64, signature string) error {
	if !c.Enabled() {
		return ErrNotConfigured
	}
	want := Sign(map[string]string{"public_id": publicID, "version": strconv.FormatInt(version, 10)}, c.apiSecret)
	if subtle.ConstantTimeCompare([]byte(want), []byte(strings.ToLower(signature))) != 1 {
		return fmt.Errorf("cloudinary upload signature does not match")
	}
	return nil
}
