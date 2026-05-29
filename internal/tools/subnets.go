package tools

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
)

func GetSubnets(svc *ec2.Client, vpcId *string) []types.Subnet {
	subnets, err := svc.DescribeSubnets(context.TODO(), &ec2.DescribeSubnetsInput{
		Filters: []types.Filter{
			{Name: aws.String("vpc-id"),
				Values: []string{*vpcId},
			},
		},
	})
	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) {
			fmt.Println(apiErr.Error())
		} else {
			fmt.Println(err.Error())
		}
	}
	return subnets.Subnets

}
