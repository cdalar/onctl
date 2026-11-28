package provideraws

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/cdalar/onctl/internal/fakeec2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The reuse-or-create helpers must not create a second VPC, internet
// gateway or security group when onctl's own already exists: duplicates
// cost money and confuse later lookups by name.

func TestCheckIfKeyPairExists(t *testing.T) {
	f, svc := fakeec2.New(t, map[string]string{
		"DescribeKeyPairs": `<keySet><item><keyName>k</keyName></item></keySet>`,
	})
	assert.True(t, checkIfKeyPairExists(svc, "k"))
	assert.Equal(t, "k", f.Field(0, "KeyName.1"))

	f.Reply("DescribeKeyPairs", `<keySet/>`)
	assert.False(t, checkIfKeyPairExists(svc, "k"))

	f.Fail("DescribeKeyPairs", "InvalidKeyPair.NotFound")
	assert.False(t, checkIfKeyPairExists(svc, "k"), "an API error means the key can't be assumed to exist")
}

func TestImportKeyPair(t *testing.T) {
	pub := filepath.Join(t.TempDir(), "id.pub")
	require.NoError(t, os.WriteFile(pub, []byte("ssh-ed25519 AAAA test"), 0o600))

	f, svc := fakeec2.New(t, map[string]string{
		"DescribeKeyPairs": `<keySet/>`,
		"ImportKeyPair":    `<keyName>k</keyName><keyFingerprint>fp</keyFingerprint>`,
	})
	ImportKeyPair(svc, "k", pub)
	assert.Equal(t, []string{"DescribeKeyPairs", "ImportKeyPair"}, f.Called())

	// Already there: no second import.
	f2, svc2 := fakeec2.New(t, map[string]string{
		"DescribeKeyPairs": `<keySet><item><keyName>k</keyName></item></keySet>`,
	})
	ImportKeyPair(svc2, "k", pub)
	assert.Equal(t, []string{"DescribeKeyPairs"}, f2.Called())
}

func TestGetDefaultVpcId(t *testing.T) {
	f, svc := fakeec2.New(t, map[string]string{
		"DescribeVpcs": `<vpcSet><item><vpcId>vpc-default</vpcId></item></vpcSet>`,
	})
	assert.Equal(t, "vpc-default", aws.ToString(GetDefaultVpcId(svc)))
	assert.Equal(t, "is-default", f.Field(0, "Filter.1.Name"))

	f.Fail("DescribeVpcs", "UnauthorizedOperation")
	assert.Nil(t, GetDefaultVpcId(svc))
}

func TestGetSecurityGroupLookups(t *testing.T) {
	f, svc := fakeec2.New(t, map[string]string{
		"DescribeSecurityGroups": `<securityGroupInfo><item><groupId>sg-1</groupId><groupName>onkube-sg-22</groupName></item></securityGroupInfo>`,
	})
	byVpc := GetSecurityGroups(svc, aws.String("vpc-1"))
	require.Len(t, byVpc, 1)
	assert.Equal(t, "sg-1", aws.ToString(byVpc[0].GroupId))
	assert.Equal(t, "vpc-id", f.Field(0, "Filter.1.Name"))
	assert.Equal(t, "vpc-1", f.Field(0, "Filter.1.Value.1"))

	byName := GetSecurityGroupByName(svc, "onkube-sg-22")
	require.Len(t, byName, 1)
	assert.Equal(t, "group-name", f.Field(1, "Filter.1.Name"))
}

func TestGetAvailabilityZones(t *testing.T) {
	f, svc := fakeec2.New(t, map[string]string{
		"DescribeAvailabilityZones": `<availabilityZoneInfo><item><zoneName>us-east-1a</zoneName></item><item><zoneName>us-east-1b</zoneName></item></availabilityZoneInfo>`,
	})
	assert.Equal(t, []string{"us-east-1a", "us-east-1b"}, getAvailabilityZones(svc))

	f.Fail("DescribeAvailabilityZones", "RequestLimitExceeded")
	assert.Nil(t, getAvailabilityZones(svc))
}

func TestCreateSubnetsOnePerZone(t *testing.T) {
	f, svc := fakeec2.New(t, map[string]string{
		"DescribeAvailabilityZones": `<availabilityZoneInfo><item><zoneName>a</zoneName></item><item><zoneName>b</zoneName></item><item><zoneName>c</zoneName></item></availabilityZoneInfo>`,
		"CreateSubnet":              `<subnet><subnetId>subnet-x</subnetId></subnet>`,
	})
	ids := createSubnets(svc, "vpc-1")
	assert.Equal(t, []string{"subnet-x", "subnet-x", "subnet-x"}, ids)

	var cidrs, zones []string
	for i, c := range f.Called() {
		if c == "CreateSubnet" {
			cidrs = append(cidrs, f.Field(i, "CidrBlock"))
			zones = append(zones, f.Field(i, "AvailabilityZone"))
		}
	}
	assert.Equal(t, []string{"10.174.0.0/20", "10.174.16.0/20", "10.174.32.0/20"}, cidrs)
	assert.Equal(t, []string{"a", "b", "c"}, zones)
}

func TestCreateVpcAndSubnetCreatesWhenMissing(t *testing.T) {
	f, svc := fakeec2.New(t, map[string]string{
		"DescribeVpcs":              `<vpcSet/>`,
		"CreateVpc":                 `<vpc><vpcId>vpc-new</vpcId></vpc>`,
		"DescribeAvailabilityZones": `<availabilityZoneInfo><item><zoneName>a</zoneName></item><item><zoneName>b</zoneName></item><item><zoneName>c</zoneName></item></availabilityZoneInfo>`,
		"CreateSubnet":              `<subnet><subnetId>subnet-new</subnetId></subnet>`,
	})
	vpcID, subnets := CreateVpcAndSubnet(svc)
	assert.Equal(t, "vpc-new", aws.ToString(vpcID))
	assert.Len(t, subnets, 3)
	assert.Contains(t, f.Called(), "CreateVpc")
}

func TestCreateVpcAndSubnetReusesExisting(t *testing.T) {
	f, svc := fakeec2.New(t, map[string]string{
		"DescribeVpcs":    `<vpcSet><item><vpcId>vpc-old</vpcId></item></vpcSet>`,
		"DescribeSubnets": `<subnetSet><item><subnetId>subnet-1</subnetId></item><item><subnetId>subnet-2</subnetId></item></subnetSet>`,
	})
	vpcID, subnets := CreateVpcAndSubnet(svc)
	assert.Equal(t, "vpc-old", aws.ToString(vpcID))
	assert.Equal(t, []string{"subnet-1", "subnet-2"}, subnets)
	assert.NotContains(t, f.Called(), "CreateVpc", "an existing onkube VPC must be reused")
}

func TestCreateInternetGateway(t *testing.T) {
	f, svc := fakeec2.New(t, map[string]string{
		"DescribeInternetGateways": `<internetGatewaySet><item><internetGatewayId>igw-old</internetGatewayId></item></internetGatewaySet>`,
	})
	assert.Equal(t, "igw-old", aws.ToString(CreateInternetGateway(svc)))
	assert.Equal(t, []string{"DescribeInternetGateways"}, f.Called())

	f2, svc2 := fakeec2.New(t, map[string]string{
		"DescribeInternetGateways": `<internetGatewaySet/>`,
		"CreateInternetGateway":    `<internetGateway><internetGatewayId>igw-new</internetGatewayId></internetGateway>`,
	})
	assert.Equal(t, "igw-new", aws.ToString(CreateInternetGateway(svc2)))
	assert.Equal(t, []string{"DescribeInternetGateways", "CreateInternetGateway"}, f2.Called())
}

func TestAttachInternetGateway(t *testing.T) {
	attached := `<internetGatewaySet><item><internetGatewayId>igw-1</internetGatewayId><attachmentSet><item><state>available</state></item></attachmentSet></item></internetGatewaySet>`
	f, svc := fakeec2.New(t, map[string]string{"DescribeInternetGateways": attached})
	AttachInternetGateway(svc, aws.String("vpc-1"), aws.String("igw-1"))
	assert.Equal(t, []string{"DescribeInternetGateways"}, f.Called(), "already attached: nothing to do")

	f2, svc2 := fakeec2.New(t, map[string]string{
		"DescribeInternetGateways": `<internetGatewaySet/>`,
		"AttachInternetGateway":    `<return>true</return>`,
	})
	AttachInternetGateway(svc2, aws.String("vpc-1"), aws.String("igw-1"))
	assert.Equal(t, []string{"DescribeInternetGateways", "AttachInternetGateway"}, f2.Called())
	assert.Equal(t, "igw-1", f2.Field(1, "InternetGatewayId"))
	assert.Equal(t, "vpc-1", f2.Field(1, "VpcId"))
}

func TestRouteTableHelpers(t *testing.T) {
	f, svc := fakeec2.New(t, map[string]string{
		"DescribeRouteTables": `<routeTableSet><item><routeTableId>rtb-1</routeTableId></item></routeTableSet>`,
		"CreateRoute":         `<return>true</return>`,
	})
	rt := DefaultRouteTable(svc, aws.String("vpc-1"))
	assert.Equal(t, "rtb-1", aws.ToString(rt))

	SetDefaultRouteToMainRouteTable(svc, rt, aws.String("igw-1"))
	assert.Equal(t, "0.0.0.0/0", f.Field(1, "DestinationCidrBlock"))
	assert.Equal(t, "rtb-1", f.Field(1, "RouteTableId"))
	assert.Equal(t, "igw-1", f.Field(1, "GatewayId"))

	f.Fail("CreateRoute", "RouteAlreadyExists")
	SetDefaultRouteToMainRouteTable(svc, rt, aws.String("igw-1")) // reports the error, doesn't panic
}

func TestCreateSecurityGroupSSH(t *testing.T) {
	f, svc := fakeec2.New(t, map[string]string{
		"DescribeSecurityGroups": `<securityGroupInfo><item><groupId>sg-ssh</groupId></item></securityGroupInfo>`,
	})
	assert.Equal(t, "sg-ssh", aws.ToString(CreateSecurityGroupSSH(svc, aws.String("vpc-1"))))
	assert.Equal(t, []string{"DescribeSecurityGroups"}, f.Called())

	f2, svc2 := fakeec2.New(t, map[string]string{
		"DescribeSecurityGroups":        `<securityGroupInfo/>`,
		"CreateSecurityGroup":           `<groupId>sg-new</groupId>`,
		"AuthorizeSecurityGroupIngress": `<return>true</return>`,
	})
	assert.Equal(t, "sg-new", aws.ToString(CreateSecurityGroupSSH(svc2, aws.String("vpc-1"))))
	assert.Equal(t, []string{"DescribeSecurityGroups", "CreateSecurityGroup", "AuthorizeSecurityGroupIngress"}, f2.Called())
	assert.Equal(t, "22", f2.Field(2, "FromPort"))
}

func TestCreateSecurityGroupForPort(t *testing.T) {
	f, svc := fakeec2.New(t, map[string]string{
		"DescribeSecurityGroups": `<securityGroupInfo><item><groupId>sg-80</groupId><groupName>onkube-sg-80</groupName></item></securityGroupInfo>`,
	})
	assert.Equal(t, "sg-80", aws.ToString(CreateSecurityGroupForPort(svc, aws.String("vpc-1"), 80)))
	assert.Equal(t, []string{"DescribeSecurityGroups"}, f.Called(), "a group for that port already exists")

	f2, svc2 := fakeec2.New(t, map[string]string{
		"DescribeSecurityGroups":        `<securityGroupInfo/>`,
		"CreateSecurityGroup":           `<groupId>sg-8080</groupId>`,
		"AuthorizeSecurityGroupIngress": `<return>true</return>`,
	})
	assert.Equal(t, "sg-8080", aws.ToString(CreateSecurityGroupForPort(svc2, aws.String("vpc-1"), 8080)))
	assert.Equal(t, "onkube-sg-8080", f2.Field(1, "GroupName"))
	assert.Equal(t, "8080", f2.Field(2, "ToPort"))
}

func TestDescribeInstance(t *testing.T) {
	f, _ := fakeec2.New(t, map[string]string{
		"DescribeInstances": `<reservationSet><item><instancesSet><item><instanceId>i-1</instanceId></item></instancesSet></item></reservationSet>`,
	})
	assert.Equal(t, "i-1", aws.ToString(DescribeInstance("i-1").InstanceId))
	assert.Equal(t, "instance-id", f.Field(0, "Filter.1.Name"))

	f.Fail("DescribeInstances", "InvalidInstanceID.NotFound")
	assert.Nil(t, DescribeInstance("i-1").InstanceId, "a failed lookup is an empty instance")
}

func TestAddSecurityGroupToInstance(t *testing.T) {
	f, svc := fakeec2.New(t, map[string]string{
		"DescribeInstances":       `<reservationSet><item><instancesSet><item><instanceId>i-1</instanceId><groupSet><item><groupId>sg-old</groupId></item></groupSet></item></instancesSet></item></reservationSet>`,
		"ModifyInstanceAttribute": `<return>true</return>`,
	})
	AddSecurityGroupToInstance(svc, aws.String("i-1"), aws.String("sg-new"))
	require.Equal(t, []string{"DescribeInstances", "ModifyInstanceAttribute"}, f.Called())
	// Modify replaces the list, so the instance's existing group has to be sent too.
	assert.Equal(t, "sg-old", f.Field(1, "GroupId.1"))
	assert.Equal(t, "sg-new", f.Field(1, "GroupId.2"))
}

func TestGetImagesSortsNewestFirst(t *testing.T) {
	f, _ := fakeec2.New(t, map[string]string{
		"DescribeImages": `<imagesSet>
			<item><imageId>ami-old</imageId><creationDate>2023-01-01T00:00:00.000Z</creationDate></item>
			<item><imageId>ami-none</imageId></item>
			<item><imageId>ami-new</imageId><creationDate>2025-01-01T00:00:00.000Z</creationDate></item>
		</imagesSet>`,
	})
	imgs, err := GetImages()
	require.NoError(t, err)
	var ids []string
	for _, i := range imgs {
		ids = append(ids, aws.ToString(i.ImageId))
	}
	assert.True(t, reflect.DeepEqual(ids, []string{"ami-new", "ami-old", "ami-none"}), "got %v", ids)
	assert.Contains(t, f.Field(0, "Filter.1.Value.1"), "ubuntu-jammy", "default image pattern")

	f.Fail("DescribeImages", "UnauthorizedOperation")
	_, err = GetImages()
	assert.Error(t, err)
}
