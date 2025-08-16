package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/hashicorp/go-cleanhttp"
)

func Main(args map[string]interface{}) map[string]interface{} {
	URL := args["URL"].(string)

	err := downloadAndUploadLatest(URL)
	if err != nil {
		return map[string]interface{}{
			"statusCode": 500,
			"body":       fmt.Sprintf("Error: %v", err),
		}
	}

	return map[string]interface{}{
		"statusCode": 200,
		"body":       filepath.Base(URL) + " uploaded successfully",
	}
}

func downloadAndUploadLatest(URL string) error {
	filename := filepath.Base(URL)

	log.Println("DOWNLOAD")

	// Download to temp file
	tmpFilePath := filepath.Join(os.TempDir(), filename)
	outFile, err := os.Create(tmpFilePath)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer func() {
		outFile.Close()
		os.Remove(tmpFilePath)
	}()

	resp, err := cleanhttp.DefaultClient().Get(URL)
	if err != nil {
		return fmt.Errorf("failed to download file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("bad HTTP status: %s", resp.Status)
	}

	log.Println("COPY")

	_, err = io.Copy(outFile, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to write to temp file: %w", err)
	}

	log.Println("UPLOAD")

	// Rewind for reading
	if _, err := outFile.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("failed to rewind temp file: %w", err)
	}

	// Setup Backblaze B2 S3 client
	endpoint := os.Getenv("B2_ENDPOINT")
	region := strings.TrimPrefix(endpoint, "s3.")
	region = strings.Split(region, ".")[0]

	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String(region),
		Endpoint:    aws.String(endpoint),
		Credentials: credentials.NewStaticCredentials(os.Getenv("B2_KEY_ID"), os.Getenv("B2_APP_KEY"), ""),
	})
	if err != nil {
		return fmt.Errorf("failed to create AWS session: %w", err)
	}

	s3Client := s3.New(sess)

	// Upload latest file
	_, err = s3Client.PutObject(&s3.PutObjectInput{
		Bucket:      aws.String(os.Getenv("B2_BUCKET")),
		Key:         aws.String(filename),
		Body:        outFile,
		ACL:         aws.String("private"),
		ContentType: aws.String("application/x-xz"),
	})
	if err != nil {
		return fmt.Errorf("upload failed: %w", err)
	}

	log.Println("DONE")
	return nil
}
