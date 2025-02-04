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
	u.Scheme = ""
	return u.String()
}

// Storage implements the ghtransport.Storage interface backed by AWS S3.
type Storage struct {
	Client *s3.Client
	Bucket string
	Prefix string
}

func (s *Storage) Get(ctx context.Context, u *url.URL) (body io.ReadCloser, header http.Header, err error) {
	out, err := s.Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.Bucket),
		Key:    aws.String(path.Join(s.Prefix, Key(*u))),
	})
	if err != nil {
		var nsk *types.NoSuchKey
		if errors.As(err, &nsk) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	headers := make(http.Header, len(out.Metadata))
	for k, v := range out.Metadata {
		headers.Set(k, v)
	}
	if out.CacheControl != nil {
		headers.Set("Cache-Control", aws.ToString(out.CacheControl))
	}
	if out.ContentDisposition != nil {
		headers.Set("Content-Disposition", aws.ToString(out.ContentDisposition))
	}
	if out.ContentEncoding != nil {
		headers.Set("Content-Encoding", aws.ToString(out.ContentEncoding))
	}
	if out.ContentLanguage != nil {
		headers.Set("Content-Language", aws.ToString(out.ContentLanguage))
	}
	if out.ContentType != nil {
		headers.Set("Content-Type", aws.ToString(out.ContentType))
	}
	return out.Body, headers, nil
}

func (s *Storage) Put(ctx context.Context, u *url.URL, body []byte, header http.Header) (err error) {
	checksum := md5.Sum(body)
	input := &s3.PutObjectInput{
		Bucket:        aws.String(s.Bucket),
		Key:           aws.String(path.Join(s.Prefix, Key(*u))),
		Body:          bytes.NewReader(body),
		ContentLength: aws.Int64(int64(len(body))),
		ContentMD5:    aws.String(base64.StdEncoding.EncodeToString(checksum[:])),
	}
	metadata := make(map[string]string, len(header))
	for key, vals := range header {
		switch key {
		case "Cache-Control":
			input.CacheControl = aws.String(vals[0])
		case "Content-Disposition":
			input.ContentDisposition = aws.String(vals[0])
		case "Content-Encoding":
			input.ContentEncoding = aws.String(vals[0])
		case "Content-Language":
			input.ContentLanguage = aws.String(vals[0])
		case "Content-Type":
			input.ContentType = aws.String(vals[0])
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
			metadata[key] = strings.Join(vals, ",")
		}
	}
	_, err = s.Client.PutObject(ctx, input)
	return err
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
