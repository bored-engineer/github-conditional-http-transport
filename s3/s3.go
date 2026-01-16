package s3storage

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// Key generates the S3 key from the URL.
var Key = func(req *http.Request) string {
	return strings.TrimPrefix(req.URL.String(), "https://")
}

// DropHeaders are the headers that are dropped before persisting the response to S3.
var DropHeaders = []string{
	"Access-Control-Allow-Origin",
	"Access-Control-Expose-Headers",
	"Content-Security-Policy",
	"Referrer-Policy",
	"Server",
	"Strict-Transport-Security",
	"X-Content-Type-Options",
	"X-Frame-Options",
	"X-Ratelimit-Limit",
	"X-Ratelimit-Remaining",
	"X-Ratelimit-Reset",
	"X-Ratelimit-Resource",
	"X-Ratelimit-Used",
	"X-Xss-Protection",
}

// Storage implements the ghtransport.Storage interface backed by AWS S3.
type Storage struct {
	Client *s3.Client
	Bucket string
	Prefix string
}

func (s *Storage) Get(ctx context.Context, req *http.Request) (*http.Response, error) {
	out, err := s.Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.Bucket),
		Key:    aws.String(path.Join(s.Prefix, Key(req))),
	})
	if err != nil {
		var nsk *types.NoSuchKey
		if errors.As(err, &nsk) {
			return nil, nil
		}
		return nil, fmt.Errorf("(*s3.Client).GetObject failed: %w", err)
	}
	headers := make(http.Header, len(out.Metadata))
	for k, v := range out.Metadata {
		headers.Set(k, v)
	}
	if value := aws.ToString(out.CacheControl); len(value) > 0 {
		headers.Set("Cache-Control", value)
	}
	if value := aws.ToString(out.ContentDisposition); len(value) > 0 {
		headers.Set("Content-Disposition", value)
	}
	if value := aws.ToString(out.ContentEncoding); len(value) > 0 {
		headers.Set("Content-Encoding", value)
	}
	if value := aws.ToString(out.ContentLanguage); len(value) > 0 {
		headers.Set("Content-Language", value)
	}
	if value := aws.ToString(out.ContentType); len(value) > 0 {
		headers.Set("Content-Type", value)
	}
	return &http.Response{
		Status:        http.StatusText(http.StatusOK),
		StatusCode:    http.StatusOK,
		Header:        headers,
		Body:          out.Body,
		ContentLength: aws.ToInt64(out.ContentLength),
	}, nil
}

func (s *Storage) Put(ctx context.Context, resp *http.Response) error {
	// Read the response body into memory
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return fmt.Errorf("(*bytes.Buffer).ReadFrom failed: %w", err)
	}
	if resp.ContentLength > 0 {
		buf.Grow(int(resp.ContentLength))
	}

	// Restore the response body
	resp.Body = io.NopCloser(bytes.NewReader(buf.Bytes()))
	resp.ContentLength = int64(buf.Len())

	// Calculate the Content-MD5 checksum
	checksum := md5.Sum(buf.Bytes())

	input := &s3.PutObjectInput{
		Bucket:        aws.String(s.Bucket),
		Key:           aws.String(path.Join(s.Prefix, Key(resp.Request))),
		Body:          &buf,
		ContentLength: aws.Int64(int64(buf.Len())),
		ContentMD5:    aws.String(base64.StdEncoding.EncodeToString(checksum[:])),
		Metadata:      make(map[string]string, len(resp.Header)),
	}
	for key, vals := range resp.Header {
		val := strings.Join(vals, ",")
		switch key {
		case "Cache-Control":
			input.CacheControl = aws.String(val)
		case "Content-Disposition":
			input.ContentDisposition = aws.String(val)
		case "Content-Encoding":
			input.ContentEncoding = aws.String(val)
		case "Content-Language":
			input.ContentLanguage = aws.String(val)
		case "Content-Type":
			input.ContentType = aws.String(val)
		default:
			if !slices.Contains(DropHeaders, key) {
				input.Metadata[key] = val
			}
		}
	}
	if _, err := s.Client.PutObject(ctx, input, s3.WithAPIOptions(
		v4.SwapComputePayloadSHA256ForUnsignedPayloadMiddleware,
	)); err != nil {
		return fmt.Errorf("(*s3.Client).PutObject failed: %w", err)
	}
	return nil
}

// New returns a new Storage for the given bucket and (optional) prefix.
func New(client *s3.Client, bucket string, prefix ...string) (*Storage, error) {
	if client == nil {
		cfg, err := config.LoadDefaultConfig(context.Background())
		if err != nil {
			return nil, fmt.Errorf("config.LoadDefaultConfig failed: %w", err)
		}
		client = s3.NewFromConfig(cfg)
	}
	return &Storage{
		Client: client,
		Bucket: bucket,
		Prefix: path.Join(prefix...),
	}, nil
}

// Must returns a new Storage for the given bucket and (optional) prefix, or panics.
func Must(client *s3.Client, bucket string, prefix ...string) *Storage {
	s, err := New(client, bucket, prefix...)
	if err != nil {
		panic(err)
	}
	return s
}
