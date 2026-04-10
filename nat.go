package main

import (
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/ec2"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type NatArgs struct {
	VpcID            pulumi.IDOutput
	PublicSubnetID   pulumi.IDOutput
	NatSgID          pulumi.IDOutput
	PrivateSubnetIDs []pulumi.IDOutput // app subnet + DB subnets
}

func NewNat(ctx *pulumi.Context, name string, args NatArgs) error {
	// fck-nat: lightweight NAT instance AMI (ARM64 for cost savings on t4g.nano).
	ami, err := ec2.LookupAmi(ctx, &ec2.LookupAmiArgs{
		MostRecent: pulumi.BoolRef(true),
		Owners:     []string{"568608671756"},
		Filters: []ec2.GetAmiFilter{
			{Name: "name", Values: []string{"fck-nat-al2023-*-arm64-*"}},
		},
	})
	if err != nil {
		return err
	}

	natInstance, err := ec2.NewInstance(ctx, name+"-nat", &ec2.InstanceArgs{
		Ami:                      pulumi.String(ami.Id),
		InstanceType:             pulumi.String("t4g.nano"),
		SubnetId:                 args.PublicSubnetID,
		VpcSecurityGroupIds:      pulumi.StringArray{args.NatSgID},
		SourceDestCheck:          pulumi.Bool(false),
		AssociatePublicIpAddress: pulumi.Bool(true),
		Tags:                     pulumi.StringMap{"Name": pulumi.String(name + "-nat")},
	})
	if err != nil {
		return err
	}

	// Private route table: all outbound traffic goes through the NAT instance.
	privateRT, err := ec2.NewRouteTable(ctx, name+"-private-rt", &ec2.RouteTableArgs{
		VpcId: args.VpcID,
		Tags:  pulumi.StringMap{"Name": pulumi.String(name + "-private-rt")},
	})
	if err != nil {
		return err
	}

	_, err = ec2.NewRoute(ctx, name+"-private-nat-route", &ec2.RouteArgs{
		RouteTableId:         privateRT.ID(),
		DestinationCidrBlock: pulumi.String("0.0.0.0/0"),
		NetworkInterfaceId:   natInstance.PrimaryNetworkInterfaceId,
	})
	if err != nil {
		return err
	}

	// Associate the private route table with app + DB subnets.
	subnetNames := []string{"app", "db-a", "db-b"}
	for i, subnetID := range args.PrivateSubnetIDs {
		_, err = ec2.NewRouteTableAssociation(ctx, name+"-private-rta-"+subnetNames[i], &ec2.RouteTableAssociationArgs{
			SubnetId:     subnetID,
			RouteTableId: privateRT.ID(),
		})
		if err != nil {
			return err
		}
	}

	return nil
}
