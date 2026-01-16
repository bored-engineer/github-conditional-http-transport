package s3storage

import (
	"bytes"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

var testURL = &url.URL{
	Scheme: "https",
	Host:   "api.github.com",
	Path:   "/users/bored-engineer",
}

var testBody = []byte(`{"login":"bored-engineer"}`)

func TestStorage(t *testing.T) {
	cfg, err := config.LoadDefaultConfig(t.Context())
	if err != nil {
		t.Fatalf("config.LoadDefaultConfig failed: %v", err)
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		// https://xuanwo.io/links/2025/02/aws_s3_sdk_breaks_its_compatible_services/
		o.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
		o.ResponseChecksumValidation = aws.ResponseChecksumValidationWhenRequired
	})

	bucketName := os.Getenv("S3_BUCKET_NAME")
	switch bucketName {
	case "":
		t.Skip("S3_BUCKET_NAME is not set, skipping test")
	case "generate":
		bucketName = "github-conditional-http-transport-test-" + strconv.Itoa(rand.Int())
		if _, err := client.CreateBucket(t.Context(), &s3.CreateBucketInput{
			Bucket: aws.String(bucketName),
		}); err != nil {
			t.Fatalf("(*s3.Client).CreateBucket failed: %v", err)
		}
		defer func() {
			// List the objects in the bucket and delete them, then delete the bucket
			objects, err := client.ListObjectsV2(t.Context(), &s3.ListObjectsV2Input{
				Bucket: aws.String(bucketName),
			})
			if err != nil {
				t.Fatalf("(*s3.Client).ListObjectsV2 failed: %v", err)
			}
			for _, obj := range objects.Contents {
				if _, err := client.DeleteObject(t.Context(), &s3.DeleteObjectInput{
					Bucket: aws.String(bucketName),
					Key:    obj.Key,
				}); err != nil {
					t.Fatalf("(*s3.Client).DeleteObject failed: %v", err)
				}
			}
			if _, err := client.DeleteBucket(t.Context(), &s3.DeleteBucketInput{
				Bucket: aws.String(bucketName),
			}); err != nil {
				t.Fatalf("(*s3.Client).DeleteBucket failed: %v", err)
			}
		}()
	default:
	}

	// Create a new storage instance with the test bucket
	storage, err := New(client, bucketName, strconv.Itoa(rand.Int()))
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	// Ensure that a request for a key not in the cache return (nil, nil)
	if missResp, err := storage.Get(t.Context(), &http.Request{
		Method: http.MethodGet,
		URL:    testURL,
	}); err != nil {
		t.Fatalf("(*Storage).Get failed: %v", err)
	} else if missResp != nil {
		t.Fatalf("(*Storage).Get returned non-nil response for invalid URL: %v", missResp)
	}

	// Ensure we can put a response into the cache
	putResp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Etag": []string{`"deadbeef"`},
		},
		Body:          io.NopCloser(bytes.NewReader(testBody)),
		ContentLength: int64(len(testBody)),
		Request: &http.Request{
			Method: http.MethodGet,
			URL:    testURL,
		},
	}
	if err := storage.Put(t.Context(), putResp); err != nil {
		t.Fatalf("(*Storage).Put failed: %v", err)
	}

	// Make sure the original body was not corrupted
	if putResp.ContentLength != int64(len(testBody)) {
		t.Fatalf("(*Storage).Put corrupted ContentLength %d, want %d", putResp.ContentLength, len(testBody))
	}
	if putBody, err := io.ReadAll(putResp.Body); err != nil {
		t.Fatalf("(*Storage).Put corrupted (*http.Response).Body.Read: %v", err)
	} else if string(putBody) != string(testBody) {
		t.Fatalf("(*Storage).Put corrupted (*http.Response).Body: %q, want %q", string(putBody), string(testBody))
	}
	if err := putResp.Body.Close(); err != nil {
		t.Fatalf("(*Storage).Put corrupted (*http.Response).Body.Close: %v", err)
	}

	// Ensure we can retrieve the response from the cache
	getResp, err := storage.Get(t.Context(), &http.Request{
		Method: http.MethodGet,
		URL:    testURL,
	})
	if err != nil {
		t.Fatalf("(*Storage).Get failed: %v", err)
	} else if getResp == nil {
		t.Fatalf("(*Storage).Get returned nil response for valid URL: %v", getResp)
	}

	// Ensure the response is correct
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("(*Storage).Get returned status code %d, want %d", getResp.StatusCode, http.StatusOK)
	}
	if getResp.Header.Get("Etag") != `"deadbeef"` {
		t.Fatalf("(*Storage).Get returned Etag header %q, want %q", getResp.Header.Get("Etag"), `"deadbeef"`)
	}
	if getResp.Body == nil {
		t.Fatalf("(*Storage).Get returned nil body")
	}

	// Ensure the body is correct
	if getResp.ContentLength != int64(len(testBody)) {
		t.Fatalf("(*Storage).Get corrupted (*http.Response).ContentLength%d, want %d", getResp.ContentLength, len(testBody))
	}
	if body, err := io.ReadAll(getResp.Body); err != nil {
		t.Fatalf("(*Storage).Get corrupted (*http.Response).Body.Read: %v", err)
	} else if string(body) != string(testBody) {
		t.Fatalf("(*Storage).Get corrupted (*http.Response).Body: %q, want %q", string(body), string(testBody))
	}
	if err := getResp.Body.Close(); err != nil {
		t.Fatalf("(*Storage).Get corrupted (*http.Response).Body.Close: %v", err)
	}
}
