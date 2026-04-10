package component

import (
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/s3"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type StorageArgs struct {
	Files []string
}

type StorageResult struct {
	BucketID  pulumi.IDOutput
	BucketArn pulumi.StringOutput
}

func NewStorage(ctx *pulumi.Context, name string, args StorageArgs) (*StorageResult, error) {
	bucket, err := s3.NewBucket(ctx, name+"-bucket", &s3.BucketArgs{
		Tags: pulumi.StringMap{"Name": pulumi.String(name + "-bucket")},
	})
	if err != nil {
		return nil, err
	}

	for _, file := range args.Files {
		_, err = s3.NewBucketObject(ctx, file, &s3.BucketObjectArgs{
			Bucket:      bucket.ID(),
			Source:      pulumi.NewFileAsset(file),
			ContentType: pulumi.String("text/html"),
		})
		if err != nil {
			return nil, err
		}
	}

	return &StorageResult{
		BucketID:  bucket.ID(),
		BucketArn: bucket.Arn,
	}, nil
}
