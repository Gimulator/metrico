// NOTE: This package is currently not being utilized in this project (`aws` package is being used instaed).
// A shameless copy of 'github.com/Gimulator/hub/pkg/s3' (with a couple of changes)

package s3

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"sigs.k8s.io/yaml"
)

var s *minio.Client

func init() {
	s3URL := os.Getenv("METRICO_S3_URL")
	s3AccessKey := os.Getenv("METRICO_S3_ACCESS_KEY")
	s3SecretKey := os.Getenv("METRICO_S3_SECRET_KEY")

	if s3URL == "" || s3AccessKey == "" || s3SecretKey == "" {
		panic("Invalid credential for S3: Set three environment variable HUB_S3_URL, HUB_S3_ACCESS_KEY, and HUB_S3_SECRET_Key for connecting to S3")
	}

	var err error
	s, err = minio.New(s3URL, &minio.Options{
		Creds:  credentials.NewStaticV4(s3AccessKey, s3SecretKey, ""),
		Secure: false,
	})

	if err != nil {
		panic(err)
	}
	// testing connection
	if _, err = s.ListBuckets(context.Background()); err != nil {
		fmt.Println("Shit")
		panic(err)
	}
}

func GetClient() *minio.Client {
	return s
}

func GetStruct(ctx context.Context, bucket, name string, i interface{}) error {
	reader, err := s.GetObject(ctx, bucket, name, minio.GetObjectOptions{})
	if err != nil {
		return err
	}
	defer reader.Close()

	content, err := io.ReadAll(reader)
	if err != nil {
		return err
	}

	if err := yaml.Unmarshal(content, i); err != nil {
		return err
	}
	return nil
}

func GetBytes(ctx context.Context, bucket, name string) ([]byte, error) {
	obj, err := s.GetObject(ctx, bucket, name, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}

	stat, err := obj.Stat()
	if err != nil {
		return nil, err
	}

	b := make([]byte, stat.Size-1)
	_, err = obj.Read(b)
	return b, err
}

func GetString(ctx context.Context, bucket, name string) (string, error) {
	bytes, err := GetBytes(ctx, bucket, name)
	return string(bytes), err
}

func PutObject(ctx context.Context, reader io.ReadCloser, bucket string, name string, contentType string) (minio.UploadInfo, error) {
	defer reader.Close()
	uploadInfo, err := s.PutObject(ctx, bucket, name, reader, -1, minio.PutObjectOptions{
		ContentType: contentType,
		// ContentType: "text/plain",
	})
	if err != nil {
		return minio.UploadInfo{}, err
	}
	return uploadInfo, nil
}

func FPutObject(ctx context.Context, filepath string, bucket string, name string, contentType string) (minio.UploadInfo, error) {
	uploadInfo, err := s.FPutObject(ctx, bucket, name, filepath, minio.PutObjectOptions{
		ContentType: contentType,
		// ContentType: "text/plain",
	})
	if err != nil {
		return minio.UploadInfo{}, err
	}
	return uploadInfo, nil
}
