package v1s3

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	v1 "github.com/w-h-a/gomento/api/domain/v1"
	"github.com/w-h-a/gomento/internal/client/uploader"
)

type v1S3Uploader struct {
	options  uploader.Options
	client   *s3.Client
	uploader *manager.Uploader
}

func (u *v1S3Uploader) Upload(ctx context.Context, fh *multipart.FileHeader) (*v1.Asset, error) {
	// use fileheader to open file contents
	file, err := fh.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open file header: %w", err)
	}
	defer file.Close()

	// calculate sha
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return nil, fmt.Errorf("hashing failed: %w", err)
	}

	sumHex := hex.EncodeToString(hasher.Sum(nil))

	// reset file pointer to the beginning so we can read again during upload
	if _, err := file.Seek(0, 0); err != nil {
		return nil, fmt.Errorf("seek failed: %w", err)
	}

	// generate path uploads/yyyy/mm/dd/hash.ext
	ext := strings.ToLower(filepath.Ext(fh.Filename))
	createdAt := time.Now().UTC()
	datePrefix := createdAt.Format("2006/01/02")
	path := fmt.Sprintf("uploads/%s/%s%s", datePrefix, sumHex, ext)

	// upload
	contentType := fh.Header.Get("Content-Type")
	if len(contentType) == 0 {
		contentType = "application/octet-stream"
	}

	out, err := u.uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(u.options.Container),
		Key:         aws.String(path),
		Body:        file,
		ContentType: aws.String(contentType),
		Metadata: map[string]string{
			"original-name": fh.Filename,
			"sha256":        sumHex,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("upload failed: %w", err)
	}

	return &v1.Asset{
		Container: u.options.Container,
		Path:      path,
		ETag:      aws.ToString(out.ETag),
		SizeBytes: fh.Size,
		MIME:      contentType,
		SHA256:    sumHex,
		CreatedAt: createdAt,
	}, nil
}

func NewV1Uploader(opts ...uploader.Option) uploader.V1Uploader {
	options := uploader.NewOptions(opts...)

	// TODO: validate options

	u := &v1S3Uploader{
		options: options,
	}

	resolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, os ...any) (aws.Endpoint, error) {
		return aws.Endpoint{URL: options.Endpoint, SigningRegion: region}, nil
	})

	cfg, err := config.LoadDefaultConfig(
		context.Background(),
		config.WithRegion(options.Region),
		config.WithEndpointResolverWithOptions(resolver),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(options.User, options.Secret, "")),
	)
	if err != nil {
		detail := "failed to initialize v1 s3 uploader"
		slog.ErrorContext(context.Background(), detail, "error", err)
		panic(detail)
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = true
	})

	u.client = client
	u.uploader = manager.NewUploader(client)

	return u
}
