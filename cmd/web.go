package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/bernard-sh/tfs/internal/web"
	"github.com/bernard-sh/tfs/internal/uploader"
	
	// Duplicate struct definition or import from ui?
	// Need to parse JSON into struct for web renderer.
	// web.GenerateHTML takes interface{}, so we need a struct that matches JSON.
	// We can reuse ui.TfPlan if exported or redefine local one.
	// Reusing ui.TfPlan requires importing "github.com/bernard-sh/tfs/internal/ui" which is fine.
	"github.com/bernard-sh/tfs/internal/ui"
)

var (
	s3Bucket   string
	gcsBucket  string
	region     string
	expiration time.Duration
)

var webCmd = &cobra.Command{
	Use:   "web <plan.binary>",
	Short: "Generate HTML report",
	Long:  `Generates a static HTML report of the terraform plan. Optionally upload to S3 or GCS.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		filename := args[0]
		
		// 1. Get JSON
		if _, err := os.Stat(filename); os.IsNotExist(err) {
			log.Fatalf("File does not exist: %s", filename)
		}
		
		tfCmd := exec.Command("terraform", "show", "-json", filename)
		output, err := tfCmd.Output()
		if err != nil {
			raw, readErr := os.ReadFile(filename)
			if readErr != nil {
				log.Fatalf("Failed to retrieve plan JSON: %v", err)
			}
			output = raw
		}
		jsonContent := string(output)

		// 2. Parse
		var plan ui.TfPlan
		dec := json.NewDecoder(strings.NewReader(jsonContent))
		dec.UseNumber()
		if err := dec.Decode(&plan); err != nil {
			log.Fatalf("Failed to parse plan JSON: %v", err)
		}

		// 3. Generate HTML
		// Use absolute path for safety or just current dir
		outputPath := "tfs.html"
		if err := web.GenerateHTML(plan, outputPath); err != nil {
			log.Fatalf("Failed to generate HTML: %v", err)
		}
		fmt.Printf("✅ Generated %s\n", outputPath)
		
		// 4. Upload Logic
		ctx := context.Background()
		fileKey := fmt.Sprintf("tfs-plan-%d.html", time.Now().Unix())
		
		if s3Bucket != "" {
			fmt.Printf("Uploading to S3 bucket: %s...\n", s3Bucket)
			u, err := uploader.NewS3Uploader(ctx, region)
			if err != nil {
				log.Fatalf("Failed to create S3 uploader: %v", err)
			}
			
			url, err := u.UploadAndPresign(ctx, s3Bucket, fileKey, outputPath, expiration)
			if err != nil {
				log.Fatalf("S3 Upload failed: %v", err)
			}
			fmt.Printf("\n🚀 Presigned URL (Expires in %s):\n%s\n", expiration, url)
		}

		if gcsBucket != "" {
			fmt.Printf("Uploading to GCS bucket: %s...\n", gcsBucket)
			u, err := uploader.NewGCSUploader(ctx)
			if err != nil {
				log.Fatalf("Failed to create GCS uploader: %v", err)
			}

			url, err := u.UploadAndSign(ctx, gcsBucket, fileKey, outputPath, expiration)
			if err != nil {
				log.Fatalf("GCS Upload failed: %v", err)
			}
			fmt.Printf("\n🚀 Signed URL (Expires in %s):\n%s\n", expiration, url)
		}
	},
}

func init() {
	rootCmd.AddCommand(webCmd)
	
	webCmd.Flags().StringVar(&s3Bucket, "s3-bucket", "", "S3 Bucket name to upload to")
	webCmd.Flags().StringVar(&gcsBucket, "gcs-bucket", "", "GCS Bucket name to upload to")
	webCmd.Flags().StringVar(&region, "region", "", "AWS Region (optional)")
	webCmd.Flags().DurationVar(&expiration, "expiration", 15*time.Minute, "Duration for the presigned URL to remain valid")
}
