package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

var (
	endpoint  = flag.String("endpoint", "http://127.0.0.1:8000", "RGW S3 endpoint")
	accessKey = flag.String("ak", "", "access key")
	secretKey = flag.String("sk", "", "secret key")
	bucket    = flag.String("bucket", "repro-multidel", "bucket name")
	region    = flag.String("region", "us-east-1", "region")

	// number of distinct objects to seed per round
	distinct = flag.Int("distinct", 8, "distinct objects seeded per round")
	// how many times each key is duplicated inside one DeleteObjects request
	dup = flag.Int("dup", 64, "duplicate count per key in a single request")
	// concurrent DeleteObjects requests in flight
	workers = flag.Int("workers", 32, "concurrent delete-multi requests")
	rounds  = flag.Int("rounds", 100000, "max rounds before giving up")
)

func newClient() *s3.Client {
	// path-style + insecure TLS for typical self-hosted RGW
	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig:     &tls.Config{InsecureSkipVerify: true},
			MaxIdleConns:        512,
			MaxIdleConnsPerHost: 512,
		},
	}
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(*region),
		config.WithHTTPClient(httpClient),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(*accessKey, *secretKey, "")),
	)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	return s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(*endpoint)
		o.UsePathStyle = true // RGW default deployments use path-style
	})
}

// seedObjects uploads `distinct` tiny objects and returns their keys.
func seedObjects(ctx context.Context, c *s3.Client, round int) []string {
	keys := make([]string, *distinct)
	var wg sync.WaitGroup
	for i := 0; i < *distinct; i++ {
		key := fmt.Sprintf("r%d/obj-%d", round, i)
		keys[i] = key
		wg.Add(1)
		go func(k string) {
			defer wg.Done()
			_, err := c.PutObject(ctx, &s3.PutObjectInput{
				Bucket: bucket,
				Key:    aws.String(k),
				Body:   strings.NewReader("xx"), // zero-length object is enough; still has an etag attr
			})
			if err != nil {
				log.Printf("put %s: %v", k, err)
			}
		}(key)
	}
	wg.Wait()
	return keys
}

// buildDeleteInput packs each key `dup` times into one request (parser does not
// dedup), so multiple concurrent RGW coroutines target the same objs_state node.
func buildDeleteInput(keys []string) *s3.DeleteObjectsInput {
	objs := make([]types.ObjectIdentifier, 0, len(keys)*(*dup))
	for _, k := range keys {
		for j := 0; j < *dup; j++ {
			objs = append(objs, types.ObjectIdentifier{Key: aws.String(k)})
		}
	}
	return &s3.DeleteObjectsInput{
		Bucket: bucket,
		Delete: &types.Delete{Objects: objs, Quiet: aws.Bool(true)},
	}
}

func main() {
	flag.Parse()
	if *accessKey == "" || *secretKey == "" {
		log.Fatal("must set -ak and -sk")
	}
	ctx := context.Background()
	c := newClient()

	// ensure bucket exists (NON-versioned: we never call PutBucketVersioning)
	_, err := c.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: bucket})
	if err != nil {
		log.Printf("create bucket (ok if exists): %v", err)
	}

	var reqCount int64
	start := time.Now()

	for round := 0; round < *rounds; round++ {
		keys := seedObjects(ctx, c, round)
		in := buildDeleteInput(keys)

		var wg sync.WaitGroup
		for w := 0; w < *workers; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := c.DeleteObjects(ctx, in)
				n := atomic.AddInt64(&reqCount, 1)
				if err != nil {
					// A crashed RGW backend surfaces as a connection
					// reset / EOF / 5xx here. That is the signal.
					log.Printf("[req %d] DeleteObjects error (possible RGW crash): %v", n, err)
				}
			}()
		}
		wg.Wait()

		if round%20 == 0 {
			rps := float64(atomic.LoadInt64(&reqCount)) / time.Since(start).Seconds()
			log.Printf("round=%d total_reqs=%d rate=%.0f req/s", round, atomic.LoadInt64(&reqCount), rps)
		}
	}
	log.Printf("done, no crash observed after %d rounds", *rounds)
}
