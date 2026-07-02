// Package storage uploads rendered artifacts to Cloudflare R2 (S3-compatible).
// Setting endpointOverride (R2_ENDPOINT) points the same S3 client at any
// other S3-compatible endpoint instead, e.g. a local MinIO container for dev.
package storage

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type R2 struct {
	client *s3.Client
	bucket string
}

func NewR2(ctx context.Context, accountID, accessKey, secretKey, bucket, endpointOverride string) (*R2, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion("auto"),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""),
		),
	)
	if err != nil {
		return nil, err
	}
	endpoint := endpointOverride
	if endpoint == "" {
		endpoint = fmt.Sprintf("https://%s.r2.cloudflarestorage.com", accountID)
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})
	return &R2{client: client, bucket: bucket}, nil
}

// UploadFile uploads a local file to the bucket under key with the given content type.
// Artifacts here are small (a few MB), so a single PutObject is sufficient.
func (r *R2) UploadFile(ctx context.Context, localPath, key, contentType string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return err
	}

	_, err = r.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(r.bucket),
		Key:           aws.String(key),
		Body:          f,
		ContentType:   aws.String(contentType),
		ContentLength: aws.Int64(stat.Size()),
	})
	return err
}

// UploadVideo uploads the final mp4 under "<id>.mp4".
func (r *R2) UploadVideo(ctx context.Context, localPath, id string) error {
	return r.UploadFile(ctx, localPath, id+".mp4", "video/mp4")
}

// UploadThumbnail uploads the thumbnail under "<id>.jpg".
func (r *R2) UploadThumbnail(ctx context.Context, localPath, id string) error {
	return r.UploadFile(ctx, localPath, id+".jpg", "image/jpeg")
}
