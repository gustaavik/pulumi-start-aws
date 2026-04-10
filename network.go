package main

import (
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/ec2"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const vpcCidr = "10.0.0.0/16"

type NetworkResult struct {
	VpcID           pulumi.IDOutput
	PublicSubnetID  pulumi.IDOutput
	PrivateSubnetID pulumi.IDOutput
	DbSubnetIDs     [2]pulumi.IDOutput
	RouterSgID      pulumi.IDOutput
	WebserverSgID   pulumi.IDOutput
	DatabaseSgID    pulumi.IDOutput
	NatSgID         pulumi.IDOutput
	ApiSgID         pulumi.IDOutput
}

func NewNetwork(ctx *pulumi.Context, name string) (*NetworkResult, error) {
	// Look up AZs in the current region.
	azs, err := aws.GetAvailabilityZones(ctx, &aws.GetAvailabilityZonesArgs{
		State: pulumi.StringRef("available"),
	})
	if err != nil {
		return nil, err
	}
	azA := azs.Names[0]
	azB := azs.Names[1]

	// VPC
	vpc, err := ec2.NewVpc(ctx, name+"-vpc", &ec2.VpcArgs{
		CidrBlock:          pulumi.String(vpcCidr),
		EnableDnsSupport:   pulumi.Bool(true),
		EnableDnsHostnames: pulumi.Bool(true),
		Tags:               pulumi.StringMap{"Name": pulumi.String(name + "-vpc")},
	})
	if err != nil {
		return nil, err
	}

	// --- Subnets ---

	publicSubnet, err := ec2.NewSubnet(ctx, name+"-public", &ec2.SubnetArgs{
		VpcId:               vpc.ID(),
		CidrBlock:           pulumi.String("10.0.1.0/24"),
		AvailabilityZone:    pulumi.String(azA),
		MapPublicIpOnLaunch: pulumi.Bool(true),
		Tags:                pulumi.StringMap{"Name": pulumi.String(name + "-public")},
	})
	if err != nil {
		return nil, err
	}

	privateSubnet, err := ec2.NewSubnet(ctx, name+"-private-app", &ec2.SubnetArgs{
		VpcId:            vpc.ID(),
		CidrBlock:        pulumi.String("10.0.2.0/24"),
		AvailabilityZone: pulumi.String(azA),
		Tags:             pulumi.StringMap{"Name": pulumi.String(name + "-private-app")},
	})
	if err != nil {
		return nil, err
	}

	dbSubnetA, err := ec2.NewSubnet(ctx, name+"-private-db-a", &ec2.SubnetArgs{
		VpcId:            vpc.ID(),
		CidrBlock:        pulumi.String("10.0.3.0/24"),
		AvailabilityZone: pulumi.String(azA),
		Tags:             pulumi.StringMap{"Name": pulumi.String(name + "-private-db-a")},
	})
	if err != nil {
		return nil, err
	}

	dbSubnetB, err := ec2.NewSubnet(ctx, name+"-private-db-b", &ec2.SubnetArgs{
		VpcId:            vpc.ID(),
		CidrBlock:        pulumi.String("10.0.4.0/24"),
		AvailabilityZone: pulumi.String(azB),
		Tags:             pulumi.StringMap{"Name": pulumi.String(name + "-private-db-b")},
	})
	if err != nil {
		return nil, err
	}

	// --- Internet Gateway + public route table ---

	igw, err := ec2.NewInternetGateway(ctx, name+"-igw", &ec2.InternetGatewayArgs{
		VpcId: vpc.ID(),
		Tags:  pulumi.StringMap{"Name": pulumi.String(name + "-igw")},
	})
	if err != nil {
		return nil, err
	}

	publicRT, err := ec2.NewRouteTable(ctx, name+"-public-rt", &ec2.RouteTableArgs{
		VpcId: vpc.ID(),
		Routes: ec2.RouteTableRouteArray{
			ec2.RouteTableRouteArgs{
				CidrBlock: pulumi.String("0.0.0.0/0"),
				GatewayId: igw.ID(),
			},
		},
		Tags: pulumi.StringMap{"Name": pulumi.String(name + "-public-rt")},
	})
	if err != nil {
		return nil, err
	}

	_, err = ec2.NewRouteTableAssociation(ctx, name+"-public-rta", &ec2.RouteTableAssociationArgs{
		SubnetId:     publicSubnet.ID(),
		RouteTableId: publicRT.ID(),
	})
	if err != nil {
		return nil, err
	}

	// --- Security Groups ---

	routerSg, err := ec2.NewSecurityGroup(ctx, name+"-tailscale-router-sg", &ec2.SecurityGroupArgs{
		VpcId:       vpc.ID(),
		Description: pulumi.String("Tailscale subnet router"),
		Ingress: ec2.SecurityGroupIngressArray{
			ec2.SecurityGroupIngressArgs{
				Protocol:    pulumi.String("udp"),
				FromPort:    pulumi.Int(41641),
				ToPort:      pulumi.Int(41641),
				CidrBlocks:  pulumi.StringArray{pulumi.String("0.0.0.0/0")},
				Description: pulumi.String("Tailscale direct connection"),
			},
		},
		Egress: ec2.SecurityGroupEgressArray{
			ec2.SecurityGroupEgressArgs{
				Protocol:   pulumi.String("-1"),
				FromPort:   pulumi.Int(0),
				ToPort:     pulumi.Int(0),
				CidrBlocks: pulumi.StringArray{pulumi.String("0.0.0.0/0")},
			},
		},
		Tags: pulumi.StringMap{"Name": pulumi.String(name + "-tailscale-router-sg")},
	})
	if err != nil {
		return nil, err
	}

	webserverSg, err := ec2.NewSecurityGroup(ctx, name+"-webserver-sg", &ec2.SecurityGroupArgs{
		VpcId:       vpc.ID(),
		Description: pulumi.String("Web server - HTTP public, SSH via Tailscale only"),
		Ingress: ec2.SecurityGroupIngressArray{
			ec2.SecurityGroupIngressArgs{
				Protocol:    pulumi.String("tcp"),
				FromPort:    pulumi.Int(22),
				ToPort:      pulumi.Int(22),
				CidrBlocks:  pulumi.StringArray{pulumi.String(vpcCidr)},
				Description: pulumi.String("SSH from VPC (Tailscale)"),
			},
			ec2.SecurityGroupIngressArgs{
				Protocol:    pulumi.String("tcp"),
				FromPort:    pulumi.Int(80),
				ToPort:      pulumi.Int(80),
				CidrBlocks:  pulumi.StringArray{pulumi.String("0.0.0.0/0")},
				Description: pulumi.String("HTTP from anywhere"),
			},
		},
		Egress: ec2.SecurityGroupEgressArray{
			ec2.SecurityGroupEgressArgs{
				Protocol:   pulumi.String("-1"),
				FromPort:   pulumi.Int(0),
				ToPort:     pulumi.Int(0),
				CidrBlocks: pulumi.StringArray{pulumi.String("0.0.0.0/0")},
			},
		},
		Tags: pulumi.StringMap{"Name": pulumi.String(name + "-webserver-sg")},
	})
	if err != nil {
		return nil, err
	}

	databaseSg, err := ec2.NewSecurityGroup(ctx, name+"-database-sg", &ec2.SecurityGroupArgs{
		VpcId:       vpc.ID(),
		Description: pulumi.String("RDS PostgreSQL - VPC-internal only"),
		Ingress: ec2.SecurityGroupIngressArray{
			ec2.SecurityGroupIngressArgs{
				Protocol:    pulumi.String("tcp"),
				FromPort:    pulumi.Int(5432),
				ToPort:      pulumi.Int(5432),
				CidrBlocks:  pulumi.StringArray{pulumi.String(vpcCidr)},
				Description: pulumi.String("PostgreSQL from VPC"),
			},
		},
		Egress: ec2.SecurityGroupEgressArray{
			ec2.SecurityGroupEgressArgs{
				Protocol:   pulumi.String("-1"),
				FromPort:   pulumi.Int(0),
				ToPort:     pulumi.Int(0),
				CidrBlocks: pulumi.StringArray{pulumi.String("0.0.0.0/0")},
			},
		},
		Tags: pulumi.StringMap{"Name": pulumi.String(name + "-database-sg")},
	})
	if err != nil {
		return nil, err
	}

	natSg, err := ec2.NewSecurityGroup(ctx, name+"-nat-sg", &ec2.SecurityGroupArgs{
		VpcId:       vpc.ID(),
		Description: pulumi.String("NAT instance - accepts all VPC traffic"),
		Ingress: ec2.SecurityGroupIngressArray{
			ec2.SecurityGroupIngressArgs{
				Protocol:    pulumi.String("-1"),
				FromPort:    pulumi.Int(0),
				ToPort:      pulumi.Int(0),
				CidrBlocks:  pulumi.StringArray{pulumi.String(vpcCidr)},
				Description: pulumi.String("All traffic from VPC"),
			},
		},
		Egress: ec2.SecurityGroupEgressArray{
			ec2.SecurityGroupEgressArgs{
				Protocol:   pulumi.String("-1"),
				FromPort:   pulumi.Int(0),
				ToPort:     pulumi.Int(0),
				CidrBlocks: pulumi.StringArray{pulumi.String("0.0.0.0/0")},
			},
		},
		Tags: pulumi.StringMap{"Name": pulumi.String(name + "-nat-sg")},
	})
	if err != nil {
		return nil, err
	}

	apiSg, err := ec2.NewSecurityGroup(ctx, name+"-api-sg", &ec2.SecurityGroupArgs{
		VpcId:       vpc.ID(),
		Description: pulumi.String("API server - VPC/Tailscale access only"),
		Ingress: ec2.SecurityGroupIngressArray{
			ec2.SecurityGroupIngressArgs{
				Protocol:    pulumi.String("tcp"),
				FromPort:    pulumi.Int(3000),
				ToPort:      pulumi.Int(3000),
				CidrBlocks:  pulumi.StringArray{pulumi.String(vpcCidr)},
				Description: pulumi.String("API from VPC (webserver + Tailscale)"),
			},
			ec2.SecurityGroupIngressArgs{
				Protocol:    pulumi.String("tcp"),
				FromPort:    pulumi.Int(22),
				ToPort:      pulumi.Int(22),
				CidrBlocks:  pulumi.StringArray{pulumi.String(vpcCidr)},
				Description: pulumi.String("SSH from VPC (Tailscale)"),
			},
		},
		Egress: ec2.SecurityGroupEgressArray{
			ec2.SecurityGroupEgressArgs{
				Protocol:   pulumi.String("-1"),
				FromPort:   pulumi.Int(0),
				ToPort:     pulumi.Int(0),
				CidrBlocks: pulumi.StringArray{pulumi.String("0.0.0.0/0")},
			},
		},
		Tags: pulumi.StringMap{"Name": pulumi.String(name + "-api-sg")},
	})
	if err != nil {
		return nil, err
	}

	return &NetworkResult{
		VpcID:           vpc.ID(),
		PublicSubnetID:  publicSubnet.ID(),
		PrivateSubnetID: privateSubnet.ID(),
		DbSubnetIDs:     [2]pulumi.IDOutput{dbSubnetA.ID(), dbSubnetB.ID()},
		RouterSgID:      routerSg.ID(),
		WebserverSgID:   webserverSg.ID(),
		DatabaseSgID:    databaseSg.ID(),
		NatSgID:         natSg.ID(),
		ApiSgID:         apiSg.ID(),
	}, nil
}
