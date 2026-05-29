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

func GetVpcs(svc *ec2.Client) *ec2.DescribeVpcsOutput {
	vpcs, err := svc.DescribeVpcs(context.TODO(), &ec2.DescribeVpcsInput{
		Filters: []types.Filter{
			{Name: aws.String("tag:Name"), Values: []string{"onkube-vpc"}}},
	})
	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) {
			fmt.Println(apiErr.Error())
		} else {
			fmt.Println(err.Error())
		}
	}
	return vpcs
}
