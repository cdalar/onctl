package provideraws

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/cdalar/onctl/internal/fakeec2"
	"github.com/stretchr/testify/assert"
)

// An API error or an empty answer must come back as a zero value the
// caller can check. Each of these used to dereference the nil or empty
// result and panic, taking the whole CLI down with a stack trace.

func TestHelpersSurviveAPIErrors(t *testing.T) {
	f, svc := fakeec2.New(t, nil)
	for _, action := range []string{
		"DescribeRouteTables", "DescribeSecurityGroups", "CreateSecurityGroup",
		"DescribeInternetGateways", "CreateInternetGateway", "CreateVpc",
		"DescribeVpcs", "DescribeInstances", "DescribeAvailabilityZones",
	} {
		f.Fail(action, "UnauthorizedOperation")
	}
	vpc := aws.String("vpc-1")

	assert.Nil(t, DefaultRouteTable(svc, vpc))
	assert.Nil(t, CreateSecurityGroupSSH(svc, vpc))
	assert.Nil(t, CreateSecurityGroupForPort(svc, vpc, 80))
	assert.Nil(t, GetSecurityGroups(svc, vpc))
	assert.Nil(t, GetSecurityGroupByName(svc, "x"))
	assert.Nil(t, CreateInternetGateway(svc))
	assert.Nil(t, createVpc(svc))
	assert.Nil(t, GetDefaultVpcId(svc))
	assert.Nil(t, DescribeInstance("i-1").InstanceId)
	AttachInternetGateway(svc, vpc, aws.String("igw-1")) // no attach attempted on a failed lookup
	assert.NotContains(t, f.Called(), "AttachInternetGateway")

	before := len(f.Called())
	id, subnets := CreateVpcAndSubnet(svc)
	assert.Nil(t, id)
	assert.Empty(t, subnets)
	assert.NotContains(t, f.Called()[before:], "CreateVpc",
		"a failed VPC lookup must not be read as 'no VPC' and create a duplicate")
}

func TestHelpersSurviveEmptyAnswers(t *testing.T) {
	_, svc := fakeec2.New(t, map[string]string{
		"DescribeRouteTables": `<routeTableSet/>`,
		"DescribeVpcs":        `<vpcSet/>`,
		"DescribeInstances":   `<reservationSet/>`,
	})
	assert.Nil(t, DefaultRouteTable(svc, aws.String("vpc-1")))
	assert.Nil(t, GetDefaultVpcId(svc))
	assert.Nil(t, DescribeInstance("i-1").InstanceId)
}

func TestCreateSubnetsStopsWhenZonesRunOut(t *testing.T) {
	f, svc := fakeec2.New(t, map[string]string{
		"DescribeAvailabilityZones": `<availabilityZoneInfo><item><zoneName>a</zoneName></item></availabilityZoneInfo>`,
		"CreateSubnet":              `<subnet><subnetId>subnet-1</subnetId></subnet>`,
	})
	assert.Equal(t, []string{"subnet-1"}, createSubnets(svc, "vpc-1"), "one zone, one subnet")
	assert.Equal(t, []string{"DescribeAvailabilityZones", "CreateSubnet"}, f.Called())
}

func TestCreateSubnetsSkipsAFailedCreate(t *testing.T) {
	f, svc := fakeec2.New(t, map[string]string{
		"DescribeAvailabilityZones": `<availabilityZoneInfo><item><zoneName>a</zoneName></item><item><zoneName>b</zoneName></item><item><zoneName>c</zoneName></item></availabilityZoneInfo>`,
	})
	f.Fail("CreateSubnet", "SubnetLimitExceeded")
	assert.Empty(t, createSubnets(svc, "vpc-1"))
}

func TestCreateVpcAndSubnetWhenVpcCreateFails(t *testing.T) {
	f, svc := fakeec2.New(t, map[string]string{"DescribeVpcs": `<vpcSet/>`})
	f.Fail("CreateVpc", "VpcLimitExceeded")
	id, subnets := CreateVpcAndSubnet(svc)
	assert.Nil(t, id)
	assert.Empty(t, subnets)
}

func TestAddSecurityGroupToInstanceWithoutGroups(t *testing.T) {
	f, svc := fakeec2.New(t, map[string]string{
		"DescribeInstances": `<reservationSet><item><instancesSet><item><instanceId>i-1</instanceId></item></instancesSet></item></reservationSet>`,
	})
	AddSecurityGroupToInstance(svc, aws.String("i-1"), aws.String("sg-new"))
	assert.NotContains(t, f.Called(), "ModifyInstanceAttribute",
		"the call replaces the group list, so it must not run without the existing group")
}
