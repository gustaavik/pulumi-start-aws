package main

import (
	"fmt"

	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/s3"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		// Create an AWS resource (S3 Bucket)
		bucket, err := s3.NewBucket(ctx, "my-bucket", nil)
		if err != nil {
			return err
		}

		// Turn the bucket into a website:
		website, err := s3.NewBucketWebsiteConfiguration(ctx, "website", &s3.BucketWebsiteConfigurationArgs{
			Bucket: bucket.ID(),
			IndexDocument: &s3.BucketWebsiteConfigurationIndexDocumentArgs{
				Suffix: pulumi.String("index.html"),
			},
		})
		if err != nil {
			return err
		}

		// Permit access control configuration:
		ownershipControls, err := s3.NewBucketOwnershipControls(ctx, "ownership-controls", &s3.BucketOwnershipControlsArgs{
			Bucket: bucket.ID(),
			Rule: &s3.BucketOwnershipControlsRuleArgs{
				ObjectOwnership: pulumi.String("ObjectWriter"),
			},
		})
		if err != nil {
			return err
		}

		// Enable public access to the website:
		publicAccessBlock, err := s3.NewBucketPublicAccessBlock(ctx, "public-access-block", &s3.BucketPublicAccessBlockArgs{
			Bucket:          bucket.ID(),
			BlockPublicAcls: pulumi.Bool(false),
		})
		if err != nil {
			return err
		}

		// Create an S3 Bucket object
		_, err = s3.NewBucketObject(ctx, "index.html", &s3.BucketObjectArgs{
			Bucket:      bucket.ID(),
			Source:      pulumi.NewFileAsset("index.html"),
			ContentType: pulumi.String("text/html"),
			Acl:         pulumi.String("public-read"),
		}, pulumi.DependsOn([]pulumi.Resource{ownershipControls, publicAccessBlock}))
		if err != nil {
			return err
		}

		// Export the name of the bucket
		ctx.Export("bucketName", bucket.ID())
		// Export the bucket's autoassigned URL:
		ctx.Export("url", website.WebsiteEndpoint.ApplyT(func(websiteEndpoint string) (string, error) {
			return fmt.Sprintf("http://%v", websiteEndpoint), nil
		}).(pulumi.StringOutput))
		return nil
	})

}
