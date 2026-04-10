package component

import (
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/rds"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type DatabaseArgs struct {
	DbSubnetIDs     [2]pulumi.IDOutput
	SecurityGroupID pulumi.IDOutput
	DbName          string
	DbUsername      string
	DbPassword      pulumi.StringOutput // from cfg.RequireSecret
}

type DatabaseResult struct {
	Endpoint pulumi.StringOutput
	Port     pulumi.IntOutput
}

func NewDatabase(ctx *pulumi.Context, name string, args DatabaseArgs) (*DatabaseResult, error) {
	subnetGroup, err := rds.NewSubnetGroup(ctx, name+"-db-subnet-group", &rds.SubnetGroupArgs{
		SubnetIds: pulumi.StringArray{
			args.DbSubnetIDs[0],
			args.DbSubnetIDs[1],
		},
		Tags: pulumi.StringMap{"Name": pulumi.String(name + "-db-subnet-group")},
	})
	if err != nil {
		return nil, err
	}

	db, err := rds.NewInstance(ctx, name+"-postgres", &rds.InstanceArgs{
		Engine:              pulumi.String("postgres"),
		EngineVersion:       pulumi.String("16"),
		InstanceClass:       pulumi.String("db.t4g.micro"),
		AllocatedStorage:    pulumi.Int(20),
		DbName:              pulumi.String(args.DbName),
		Username:            pulumi.String(args.DbUsername),
		Password:            args.DbPassword,
		DbSubnetGroupName:   subnetGroup.Name,
		VpcSecurityGroupIds: pulumi.StringArray{args.SecurityGroupID},
		PubliclyAccessible:  pulumi.Bool(false),
		SkipFinalSnapshot:   pulumi.Bool(true),
		Tags:                pulumi.StringMap{"Name": pulumi.String(name + "-postgres")},
	})
	if err != nil {
		return nil, err
	}

	return &DatabaseResult{
		Endpoint: db.Endpoint,
		Port:     db.Port,
	}, nil
}
