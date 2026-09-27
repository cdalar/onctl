package cloud

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/stretchr/testify/assert"
)

func TestMapAwsServer(t *testing.T) {
	launched := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	inst := types.Instance{
		InstanceId:       aws.String("i-0abc"),
		InstanceType:     types.InstanceTypeT2Micro,
		PublicIpAddress:  aws.String("1.2.3.4"),
		PrivateIpAddress: aws.String("10.0.0.5"),
		LaunchTime:       &launched,
		State:            &types.InstanceState{Name: types.InstanceStateNameRunning},
		Placement:        &types.Placement{AvailabilityZone: aws.String("eu-central-1a")},
		Tags: []types.Tag{
			{Key: aws.String("Owner"), Value: aws.String("someone")},
			{Key: aws.String("Name"), Value: aws.String("web")},
		},
	}

	vm := mapAwsServer(inst)
	assert.Equal(t, "aws", vm.Provider)
	assert.Equal(t, "i-0abc", vm.ID)
	// onctl identifies VMs by name, which on AWS lives only in the Name tag.
	assert.Equal(t, "web", vm.Name)
	assert.Equal(t, "1.2.3.4", vm.IP)
	assert.Equal(t, "10.0.0.5", vm.PrivateIP)
	assert.Equal(t, "t2.micro", vm.Type)
	assert.Equal(t, "running", vm.Status)
	assert.Equal(t, launched, vm.CreatedAt)
	assert.Equal(t, "eu-central-1a", vm.Location)
	assert.Equal(t, "N/A", vm.Cost.Currency)
}

// Stopped instances have no public IP and untagged ones have no name; mapping
// must not dereference the nil pointers AWS returns for those.
func TestMapAwsServer_MissingOptionalFields(t *testing.T) {
	launched := time.Now()
	inst := types.Instance{
		InstanceId:   aws.String("i-0def"),
		InstanceType: types.InstanceTypeT3Small,
		LaunchTime:   &launched,
		State:        &types.InstanceState{Name: types.InstanceStateNameStopped},
		Placement:    &types.Placement{AvailabilityZone: aws.String("us-east-1b")},
	}

	var vm Vm
	assert.NotPanics(t, func() { vm = mapAwsServer(inst) })
	assert.Empty(t, vm.Name)
	assert.Empty(t, vm.IP)
	assert.Empty(t, vm.PrivateIP)
	assert.Equal(t, "stopped", vm.Status)
}
