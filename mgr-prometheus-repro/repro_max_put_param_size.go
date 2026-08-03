// go run repro_max_put_param_size.go -endpoint http://10.16.21.12:80 -ak testy -sk testy -bucket test
package main

// 复现 CompleteMultipartUpload 触发 rgw_max_put_param_size (默认 1MB) 限制
//
// 原理: rgw_rest.cc:1519 检查 Content-Length > rgw_max_put_param_size → 416 InvalidRange
//
// 方案: 9000 个假分段 × 122 字节 ETag ≈ 1.6MB XML，SDK 正常发送 Content-Length
//       分段数远低于 rgw_multipart_part_upload_limit (10000)，只触发 XML 大小限制

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	smithy "github.com/aws/smithy-go"
)

func main() {
	endpoint := flag.String("endpoint", "http://localhost:80", "RGW endpoint")
	bucket   := flag.String("bucket", "test", "bucket name")
	ak       := flag.String("ak", "", "access key")
	sk       := flag.String("sk", "", "secret key")
	key      := flag.String("key", "repro-max-put-param-size", "object key")
	numParts := flag.Int("parts", 9000, "fake parts count; keep <10000 to avoid part count limit")
	padLen   := flag.Int("pad", 90, "extra padding chars per ETag; 9000×(32+90+overhead)≈1.6MB")
	flag.Parse()

	if *ak == "" || *sk == "" {
		log.Fatal("-ak and -sk are required")
	}

	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(*ak, *sk, "")),
		config.WithEndpointResolverWithOptions(
			aws.EndpointResolverWithOptionsFunc(func(service, _ string, options ...interface{}) (aws.Endpoint, error) {
				return aws.Endpoint{URL: *endpoint, SigningRegion: "us-east-1"}, nil
			}),
		),
	)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) { o.UsePathStyle = true })
	ctx := context.TODO()

	createResp, err := client.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{
		Bucket: bucket,
		Key:    key,
	})
	if err != nil {
		log.Fatalf("CreateMultipartUpload: %v", err)
	}
	uploadID := *createResp.UploadId
	log.Printf("uploadId: %s", uploadID)

	// 每个假 ETag = 32 位 hex + padding，共 32+pad 字节
	// 每条 XML 约 56+(32+pad) 字节 (不含数字位数)
	// 9000 × (56+32+90) = 9000 × 178 ≈ 1.6MB > 1MB 默认限制
	pad := strings.Repeat("0", *padLen)
	fakeParts := make([]types.CompletedPart, *numParts)
	for i := range fakeParts {
		pn := int32(i + 1)
		fakeParts[i] = types.CompletedPart{
			PartNumber: aws.Int32(pn),
			ETag:       aws.String(fmt.Sprintf("%032x%s", pn, pad)),
		}
	}

	etagLen := 32 + *padLen
	xmlEst := 53 + *numParts*(56+etagLen)
	const xmlLimit = 1048576
	log.Printf("parts: %d | etag: %d chars | estimated XML: %d bytes (%.1f KB) | limit: %d bytes",
		*numParts, etagLen, xmlEst, float64(xmlEst)/1024, xmlLimit)

	start := time.Now()
	_, completeErr := client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket:   bucket,
		Key:      key,
		UploadId: aws.String(uploadID),
		MultipartUpload: &types.CompletedMultipartUpload{
			Parts: fakeParts,
		},
	})
	latency := time.Since(start)

	if completeErr != nil {
		var apiErr smithy.APIError
		if errors.As(completeErr, &apiErr) {
			log.Printf("%s | latency=%s", apiErr.ErrorCode(), latency.Round(time.Millisecond))
			if apiErr.ErrorCode() == "InvalidRange" && latency < 5*time.Millisecond {
				log.Printf("*** REPRODUCED: 416 InvalidRange, rgw_max_put_param_size triggered ***")
				log.Printf("    fix: ceph config set client.rgw rgw_max_put_param_size 104857600")
			} else if apiErr.ErrorCode() == "InvalidRange" {
				log.Printf("NOTE: latency>0 means body was read — likely rgw_multipart_part_upload_limit, not XML size")
			} else {
				log.Printf("*** rgw_max_put_param_size check passed (got %s, not InvalidRange) ***", apiErr.ErrorCode())
			}
		} else {
			log.Printf("error: %v | latency=%s", completeErr, latency.Round(time.Millisecond))
		}
	} else {
		log.Printf("CompleteMultipartUpload succeeded | latency=%s", latency.Round(time.Millisecond))
	}

	client.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{
		Bucket:   bucket,
		Key:      key,
		UploadId: aws.String(uploadID),
	})
}
