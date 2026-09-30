package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"clientesFrecuentes/internal/config"
	"clientesFrecuentes/internal/model"
	"github.com/google/uuid"
)

func TestRegistrationRequiresSeparateNamesBeforePersistence(t *testing.T) {
	s := &Service{Config: config.Config{DemoSignupEnabled: true}}
	for _, tc := range []struct{ name, last string }{{"Gabriel", ""}, {"", "Gonzalez"}, {"123", "---"}, {"Gabriel", "  "}, {strings.Repeat("a", 121), "Gonzalez"}} {
		if _, err := s.RegisterCustomer(context.Background(), model.RegisterCustomerRequest{Name: tc.name, LastName: tc.last, Email: "gg@example.test", Password: "long-password"}); !errors.Is(err, ErrRegistrationProfileRequired) {
			t.Fatalf("customer %q/%q: %v", tc.name, tc.last, err)
		}
		if _, err := s.RegisterDemoMerchant(context.Background(), uuid.NewString(), uuid.NewString(), model.RegisterDemoMerchantRequest{OwnerName: tc.name, OwnerLastName: tc.last, Email: "gg@example.test", Password: "long-password"}); !errors.Is(err, ErrRegistrationProfileRequired) {
			t.Fatalf("merchant: %v", err)
		}
		if _, err := s.RegisterInvitation(context.Background(), strings.Repeat("a", 40), model.RegisterInvitationRequest{Name: tc.name, LastName: tc.last, Password: "long-password"}); !errors.Is(err, ErrRegistrationProfileRequired) {
			t.Fatalf("invitation: %v", err)
		}
	}
	n, l, err := registrationNames("  María José  ", " Pérez López ")
	if err != nil || n != "María José" || l != "Pérez López" {
		t.Fatalf("normalized %q/%q %v", n, l, err)
	}
}
