package cloudinary

import (
	"testing"
	"time"
)

func TestSignMatchesCloudinaryExample(t *testing.T) {
	// Example from Cloudinary's "Generating authentication signatures" guide.
	got := Sign(map[string]string{"eager": "w_400,h_300,c_pad|w_260,h_200,c_crop", "public_id": "sample_image",
		"timestamp": "1315060510"}, "abcd")
	if got != "bfd09f95f331f558cbd1320e67aa8d488770583e" {
		t.Fatalf("signature = %s", got)
	}
}

func TestSignUploadCoversEverySignedField(t *testing.T) {
	client := New("demo", "key", "secret")
	client.now = func() time.Time { return time.Unix(1_700_000_000, 0) }
	got, err := client.SignUpload("ara/test/members/1/passport")
	if err != nil {
		t.Fatal(err)
	}
	want := Sign(map[string]string{
		"allowed_formats": "jpg,png", "invalidate": "true", "overwrite": "true",
		"public_id": "ara/test/members/1/passport", "timestamp": "1700000000",
	}, "secret")
	if got.Signature != want || got.CloudName != "demo" || got.APIKey != "key" || got.Timestamp != 1_700_000_000 {
		t.Fatalf("signature = %+v", got)
	}
	if _, err := New("", "", "").SignUpload("x"); err != ErrNotConfigured {
		t.Fatal("unconfigured client signed an upload")
	}
}

func TestVerifyUpload(t *testing.T) {
	client := New("demo", "key", "secret")
	good := Sign(map[string]string{"public_id": "ara/test/members/1/passport", "version": "42"}, "secret")
	if err := client.VerifyUpload("ara/test/members/1/passport", 42, good); err != nil {
		t.Fatalf("valid response rejected: %v", err)
	}
	for name, check := range map[string]func() error{
		"other version": func() error { return client.VerifyUpload("ara/test/members/1/passport", 43, good) },
		"other id":      func() error { return client.VerifyUpload("ara/test/members/2/passport", 42, good) },
		"other secret":  func() error { return New("demo", "key", "other").VerifyUpload("ara/test/members/1/passport", 42, good) },
	} {
		if check() == nil {
			t.Fatalf("%s accepted", name)
		}
	}
}
