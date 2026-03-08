package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNew_Cloudflare(t *testing.T) {
	p := New("cloudflare")
	assert.NotNil(t, p)
	_, ok := p.(*CloudFlareService)
	assert.True(t, ok)
}

func TestNew_Unknown(t *testing.T) {
	p := New("unknown")
	assert.Nil(t, p)
}

func TestNew_Empty(t *testing.T) {
	p := New("")
	assert.Nil(t, p)
}

func TestSetRecordRequest(t *testing.T) {
	req := &SetRecordRequest{
		Subdomain: "test.example.com",
		Ipaddress: "1.2.3.4",
	}
	assert.Equal(t, "test.example.com", req.Subdomain)
	assert.Equal(t, "1.2.3.4", req.Ipaddress)
}

func TestNewCloudFlareService_NoToken(t *testing.T) {
	t.Setenv("CLOUDFLARE_API_TOKEN", "")
	svc := NewCloudFlareService()
	assert.NotNil(t, svc)
	assert.Equal(t, "", svc.CLOUDFLARE_API_TOKEN)
}

func TestNewCloudFlareService_WithToken(t *testing.T) {
	t.Setenv("CLOUDFLARE_API_TOKEN", "test-token-12345")
	svc := NewCloudFlareService()
	assert.NotNil(t, svc)
	assert.Equal(t, "test-token-12345", svc.CLOUDFLARE_API_TOKEN)
}

func TestCheckEnv_NoToken(t *testing.T) {
	t.Setenv("CLOUDFLARE_API_TOKEN", "")
	t.Setenv("CLOUDFLARE_ZONE_ID", "")
	svc := &CloudFlareService{}
	err := svc.CheckEnv()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "CLOUDFLARE_API_TOKEN is not set")
}

func TestCheckEnv_NoZoneID(t *testing.T) {
	t.Setenv("CLOUDFLARE_API_TOKEN", "test-token")
	t.Setenv("CLOUDFLARE_ZONE_ID", "")
	svc := &CloudFlareService{}
	err := svc.CheckEnv()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "CLOUDFLARE_ZONE_ID is not set")
}

func TestCheckEnv_AllSet(t *testing.T) {
	t.Setenv("CLOUDFLARE_API_TOKEN", "test-token")
	t.Setenv("CLOUDFLARE_ZONE_ID", "zone-123")
	svc := &CloudFlareService{}
	err := svc.CheckEnv()
	assert.NoError(t, err)
}
