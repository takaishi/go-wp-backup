package main

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/pkg/errors"
	"log"
	"os"
	"time"
)

type AWSBackuper struct {
	source string
	aws    *AWSConfig
}

func (b *AWSBackuper) Run() error {
	n := time.Now()
	dumpSubdir := n.Format("20060102T150405")
	bucket := b.aws.Bucket

	log.Printf("Start backup to s3://%s/%s\n", bucket, dumpSubdir)

	log.Printf("Start upload backups to S3\n")
	err := b.backupToS3(b.source, dumpSubdir)
	if err != nil {
		log.Fatalf("Failed to upload to s3: %s\n", err)
		return err
	}

	log.Printf("Start rotate backups\n")
	err = b.rotateBackup()
	if err != nil {
		log.Fatalf("Failed to rotate backups: %s\n", err)
		return err
	}

	log.Printf("Finish backup to s3://%s/%s\n", bucket, dumpSubdir)

	return nil
}

func (b *AWSBackuper) backupToS3(dumpDir string, dumpSubdir string) error {
	accessKeyID := b.aws.AccessKeyID
	secretAccessKey := b.aws.SecretAccessKey
	region := b.aws.Region
	bucket := b.aws.Bucket

	sess, err := session.NewSession(&aws.Config{
		Credentials: credentials.NewStaticCredentials(accessKeyID, secretAccessKey, ""),
		Region:      aws.String(region),
	})
	if err != nil {
		return errors.Wrap(err, "Failed to create session")
	}
	cli := s3.New(sess)

	for _, name := range []string{"wordpress.sql", "wordpress.zip"} {
		err := b.uploadToS3(cli, fmt.Sprintf("%s/%s", dumpDir, name), bucket, fmt.Sprintf("%s/%s", dumpSubdir, name))
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("Failed to upload %s to s3", name))
		}
	}
	return nil

}

func (b *AWSBackuper) uploadToS3(cli *s3.S3, path string, bucket string, key string) error {
	f, err := os.Open(path)
	if err != nil {
		return errors.Wrap(err, "Failed to open file")
	}
	defer f.Close()

	_, err = cli.PutObject(&s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   f,
	})

	if err != nil {
		return errors.Wrap(err, "Failed to PutObject")
	}

	return nil
}

func (b *AWSBackuper) rotateBackup() error {
	backup_size := 3
	accessKeyID := b.aws.AccessKeyID
	secretAccessKey := b.aws.SecretAccessKey
	region := b.aws.Region
	bucket := b.aws.Bucket

	sess, err := session.NewSession(&aws.Config{
		Credentials: credentials.NewStaticCredentials(accessKeyID, secretAccessKey, ""),
		Region:      aws.String(region),
	})
	if err != nil {
		return errors.Wrap(err, "Failed to create session")
	}

	cli := s3.New(sess)

	keys, err := b.getDeletePrefixes(cli, bucket, backup_size)
	if err != nil {
		return errors.Wrap(err, "Failed to get delete prefixes")
	}

	for _, k := range keys {
		result, err := cli.ListObjectsV2(&s3.ListObjectsV2Input{
			Bucket:    aws.String(bucket),
			Prefix:    aws.String(fmt.Sprintf("%s", k)),
			Delimiter: aws.String("/"),
		})
		if err != nil {
			return errors.Wrap(err, "Failed to list object")
		}
		for _, o := range result.Contents {
			err := b.deleteObject(cli, bucket, *o.Key)
			if err != nil {
				return errors.Wrap(err, "Failed to deleteObject")
			}
		}
	}
	return nil
}

func (b *AWSBackuper) getDeletePrefixes(cli *s3.S3, bucket string, backup_size int) ([]string, error) {
	var keys []string
	result, err := cli.ListObjectsV2(&s3.ListObjectsV2Input{
		Bucket:    aws.String(bucket),
		Delimiter: aws.String("/"),
	})

	if err != nil {
		return nil, errors.Wrap(err, "Failed to list object")
	}

	for _, k := range result.CommonPrefixes[0 : len(result.CommonPrefixes)-backup_size] {
		keys = append(keys, *k.Prefix)
	}

	return keys, nil
}

func (b *AWSBackuper) deleteObject(cli *s3.S3, bucket string, key string) error {
	log.Printf("Delete Object: s3://%s/%s\n", bucket, key)

	_, err := cli.DeleteObject(&s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return errors.Wrap(err, "Failed to delete object")
	}
	return nil
}
