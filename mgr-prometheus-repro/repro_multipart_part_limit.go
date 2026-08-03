package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

const (
	// 固定分段大小为 5MB（Ceph RGW 限制为 10000 个分段）
	partSize   int64 = 5 * 1024 * 1024
	concurrent       = 50
)

// partTask 描述一个待上传的分段
type partTask struct {
	partNumber int32
	offset     int64
	size       int64
}

func main() {
	// ========== 1. 配置参数 ==========
	endpoint := flag.String("endpoint", "http://localhost:80", "RGW endpoint")
	bucketName := flag.String("bucket", "test", "bucket name")
	accessKey := flag.String("ak", "", "access key")
	secretKey := flag.String("sk", "", "secret key")
	objectKey := flag.String("key", "large-file-120g.dat", "object key")
	filePath := flag.String("file", "/tmp/bigfile", "file wanted to upload")
	// numParts := flag.Int("parts", 9000, "fake parts count; keep <10000 to avoid part count limit")
	// padLen   := flag.Int("pad", 90, "extra padding chars per ETag; 9000×(32+90+overhead)≈1.6MB")
	flag.Parse()

	if *accessKey == "" || *secretKey == "" {
		log.Fatal("-ak and -sk are required")
	}
	// endpoint := "http://10.16.21.12:80" // 替换为你的 Ceph RGW 端点
	// accessKey := "testy"
	// secretKey := "testy"
	// bucketName := "test"
	// objectKey := "large-file-120g.dat"
	// filePath := "/tmp/bigfile" // 替换为实际文件路径

	// ==================== 2. 读取文件信息 ====================
	fileInfo, err := os.Stat(*filePath)
	if err != nil {
		panic(fmt.Sprintf("stat file: %v", err))
	}
	fileSize := fileInfo.Size()
	fmt.Printf("File size: %d bytes (%.2f GB)\n", fileSize, float64(fileSize)/(1024*1024*1024))

	estimatedParts := fileSize / partSize
	if fileSize%partSize != 0 {
		estimatedParts++
	}
	fmt.Printf("Part size: %d bytes (5 MB)\n", partSize)
	fmt.Printf("Estimated parts: %d\n", estimatedParts)
	if estimatedParts > 10000 {
		fmt.Printf("⚠️  WARNING: Parts (%d) exceeds 10000 limit. Upload will fail after part 10001.\n", estimatedParts)
	}

	// ==================== 3. 创建 AWS Config ====================
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion("us-east-1"),
		config.WithEndpointResolverWithOptions(
			aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
				return aws.Endpoint{URL: *endpoint, SigningRegion: "us-east-1"}, nil
			}),
		),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(*accessKey, *secretKey, "")),
	)
	if err != nil {
		panic(fmt.Sprintf("load config: %v", err))
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = true
	})

	// ==================== 4. 初始化分段上传 ====================
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	createResp, err := client.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{
		Bucket: bucketName,
		Key:    objectKey,
	})
	if err != nil {
		panic(fmt.Sprintf("CreateMultipartUpload: %v", err))
	}
	uploadID := *createResp.UploadId
	fmt.Printf("UploadID: %s\n", uploadID)

	// ==================== 5. 生成任务列表 ====================
	var tasks []partTask
	var offset int64 = 0
	var partNum int32 = 1
	for offset < fileSize {
		size := partSize
		if offset+size > fileSize {
			size = fileSize - offset
		}
		tasks = append(tasks, partTask{
			partNumber: partNum,
			offset:     offset,
			size:       size,
		})
		offset += size
		partNum++
	}
	fmt.Printf("Total parts to upload: %d\n", len(tasks))

	// ==================== 6. 并发上传 ====================
	var wg sync.WaitGroup
	taskCh := make(chan partTask, len(tasks))
	resultCh := make(chan types.CompletedPart, len(tasks))
	errCh := make(chan error, 1) // 只记录第一个错误

	// 启动 worker
	for i := 0; i < concurrent; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range taskCh {
				// 检查是否已被取消
				select {
				case <-ctx.Done():
					return
				default:
				}

				// 读取文件片段
				buf := make([]byte, task.size)
				file, err := os.Open(*filePath)
				if err != nil {
					select {
					case errCh <- fmt.Errorf("open file: %v", err):
					default:
					}
					cancel()
					return
				}
				_, err = file.ReadAt(buf, task.offset)
				file.Close()
				if err != nil {
					select {
					case errCh <- fmt.Errorf("read at %d: %v", task.offset, err):
					default:
					}
					cancel()
					return
				}

				// 上传分段
				uploadResp, err := client.UploadPart(ctx, &s3.UploadPartInput{
					Bucket:     bucketName,
					Key:        objectKey,
					PartNumber: aws.Int32(task.partNumber),
					UploadId:   aws.String(uploadID),
					Body:       bytes.NewReader(buf),
				})
				if err != nil {
					select {
					case errCh <- fmt.Errorf("part %d: %v", task.partNumber, err):
					default:
					}
					cancel()
					return
				}

				// 成功
				resultCh <- types.CompletedPart{
					ETag:       uploadResp.ETag,
					PartNumber: aws.Int32(task.partNumber),
				}
				fmt.Printf("✓ Part %d uploaded (%d bytes)\n", task.partNumber, task.size)
			}
		}()
	}

	// 发送任务
	go func() {
		defer close(taskCh)
		for _, t := range tasks {
			select {
			case <-ctx.Done():
				return
			case taskCh <- t:
			}
		}
	}()

	wg.Wait()
	close(resultCh)

	var parts []types.CompletedPart
	for part := range resultCh {
		parts = append(parts, part)
	}
	var uploadErr error
	select {
	case err := <-errCh:
		uploadErr = err
	default:
	}

	// ==================== 7. 处理结果 ====================
	abortCtx := context.Background()
	abort := func() {
		_, _ = client.AbortMultipartUpload(abortCtx, &s3.AbortMultipartUploadInput{
			Bucket:   bucketName,
			Key:      objectKey,
			UploadId: aws.String(uploadID),
		})
	}

	if uploadErr != nil {
		fmt.Printf("❌ Upload failed (expected if >10000 parts): %v\n", uploadErr)
		abort()
		fmt.Println("✅ Simulation successful: exceeded part limit, upload aborted.")
		return
	}

	// 如果成功（理论上不会发生，因为文件太大），则完成合并
	fmt.Printf("All %d parts uploaded successfully (unexpected). Completing...\n", len(parts))
	_, err = client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket:   bucketName,
		Key:      objectKey,
		UploadId: aws.String(uploadID),
		MultipartUpload: &types.CompletedMultipartUpload{
			Parts: parts,
		},
	})
	if err != nil {
		abort()
		panic(fmt.Sprintf("CompleteMultipartUpload: %v", err))
	}
	fmt.Println("✅ Upload completed successfully (this should not happen for >10000 parts).")
}
