package daemon

import (
	"testing"
	"time"
)

func TestManagementCapabilityIsSingleUseAndClearsSecret(t *testing.T) {
	secret := []byte("not-a-real-secret")
	service := &Service{management: map[string]*profileManagementSession{
		"capability": {profile: "example", secret: secret, expiresAt: time.Now().Add(time.Minute)},
	}}

	if err := service.consumeManagementSession("example", "capability"); err != nil {
		t.Fatal(err)
	}
	for index, value := range secret {
		if value != 0 {
			t.Fatalf("secret byte %d was not cleared", index)
		}
	}
	if err := service.consumeManagementSession("example", "capability"); err == nil {
		t.Fatal("reused management capability was accepted")
	}
}

func TestExpiredManagementCapabilityIsRejected(t *testing.T) {
	service := &Service{management: map[string]*profileManagementSession{
		"expired": {profile: "example", secret: []byte("not-a-real-secret"), expiresAt: time.Now().Add(-time.Second)},
	}}

	if err := service.consumeManagementSession("example", "expired"); err == nil {
		t.Fatal("expired management capability was accepted")
	}
	if len(service.management) != 0 {
		t.Fatal("expired management capability was not removed")
	}
}
