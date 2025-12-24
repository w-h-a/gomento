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
	"github.com/w-h-a/gomento/internal/client/filer"
)

type v1S3Filer struct {
	options   filer.Options
	client    *s3.Client
	uploader  *manager.Uploader
	presigner *s3.PresignClient
}

func (f *v1S3Filer) Upload(ctx context.Context, fh *multipart.FileHeader) (*v1.Asset, error) {
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
	datePrefix := time.Now().UTC().Format("2006/01/02")
	path := fmt.Sprintf("uploads/%s/%s%s", datePrefix, sumHex, ext)

	// upload
	contentType := fh.Header.Get("Content-Type")
	if len(contentType) == 0 {
		contentType = "application/octet-stream"
	}

	out, err := f.uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(f.options.Container),
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
		Container: f.options.Container,
		Path:      path,
		ETag:      aws.ToString(out.ETag),
		SizeBytes: fh.Size,
		MIME:      contentType,
		SHA256:    sumHex,
	}, nil
}

func (f *v1S3Filer) PresignGet(ctx context.Context, path string, expire time.Duration) (string, error) {
	req, err := f.presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(f.options.Container),
		Key:    aws.String(path),
	}, func(po *s3.PresignOptions) {
		po.Expires = expire
	})
	if err != nil {
		return "", fmt.Errorf("failed to presign get: %w", err)
	}

	return req.URL, nil
}

func NewV1Filer(opts ...filer.Option) filer.V1Filer {
	options := filer.NewOptions(opts...)

	// TODO: validate options

	f := &v1S3Filer{
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

	f.client = client
	f.uploader = manager.NewUploader(client)
	f.presigner = s3.NewPresignClient(client)

	return f
}
