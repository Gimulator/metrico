package aws

import (
	"context"
	"errors"
	"io"

	"github.com/aws/aws-sdk-go/aws"
	cr "github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

var client *s3.S3
var _init bool

func Init(url string, id string, secret string) {
	sess := session.Must(session.NewSession())
	creds := cr.NewStaticCredentials(id, secret, "")
	client = s3.New(sess, &aws.Config{
		Credentials: creds,
		Endpoint:    aws.String(url),
		Region:      aws.String("us-east-1"),
		DisableSSL:  aws.Bool(true),
	})
	_init = true
}

func PutObject(ctx context.Context, bucket string, key string, reader io.ReadSeeker) error {
	if _init {
		_, err := client.PutObjectWithContext(context.Background(), &s3.PutObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
			Body:   reader,
		})
		return err
	}
	return errors.New("AWS package is not initialized yet.")
}

// TODO: implement other methods such as FPutObject, GetObject, ...
