package s3storage

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// Key generates the S3 key from the URL.
var Key = func(u url.URL) string {
	return strings.TrimPrefix(u.String(), "https://")
}

// Storage implements the ghtransport.Storage interface backed by AWS S3.
type Storage struct {
	Client *s3.Client
	Bucket string
	Prefix string
}

func (s *Storage) Get(ctx context.Context, u *url.URL) (*http.Response, error) {
	out, err := s.Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.Bucket),
		Key:    aws.String(path.Join(s.Prefix, Key(*u))),
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

func (s *Storage) Put(ctx context.Context, u *url.URL, resp *http.Response) error {
	input := &s3.PutObjectInput{
		Bucket:        aws.String(s.Bucket),
		Key:           aws.String(path.Join(s.Prefix, Key(*u))),
		Body:          resp.Body,
		ContentLength: aws.Int64(resp.ContentLength),
		// ContentMD5:    aws.String(base64.StdEncoding.EncodeToString(checksum[:])),
	}
	metadata := make(map[string]string, len(resp.Header))
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
		case "Access-Control-Allow-Origin",
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
			"X-Xss-Protection":
			// Drop these headers, they're just noise.
		default:
			metadata[key] = val
		}
	}
	if _, err := s.Client.PutObject(ctx, input); err != nil {
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
