package service

import (
	"context"
	"errors"
	"testing"

	"clientesFrecuentes/internal/model"
)

func TestUpdateBrandRejectsUnknownCardDesignValues(t *testing.T) {
	svc := &Service{}
	unknownTemplate := "UNKNOWN_TEMPLATE"
	if _, err := svc.UpdateBrand(context.Background(), 1, 1, 1, model.UpdateBrandRequest{CardTemplate: &unknownTemplate}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("unknown template error=%v", err)
	}
	unsafeIcon := "../../logo"
	if _, err := svc.UpdateBrand(context.Background(), 1, 1, 1, model.UpdateBrandRequest{RewardImage: &unsafeIcon}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("unsafe reward icon error=%v", err)
	}
}

func TestCardDesignValidationMatchesEveryPublishedTemplate(t *testing.T) {
	publishedTemplates := []string{
		"COMIC_HQ", "MINIMAL_PRO", "POP_BADGE", "PREMIUM_GOLD_CHECKS", "DARK_LUXURY_CHECKS",
		"PREMIUM_GOLD", "DARK_LUXURY", "POP_HEADER", "COMIC_HQ_POINTS", "MINIMAL_PRO_POINTS",
		"POP_BADGE_POINTS", "NEON_PULSE", "SUNSET_GRADIENT",
		"EDITORIAL_CHECKS", "TICKET_CHECKS", "ORBIT_CHECKS", "SPLIT_CHECKS",
		"EDITORIAL_POINTS", "TICKET_POINTS", "ORBIT_POINTS", "SPLIT_POINTS",
		"EMBLEM_CHECKS", "MARQUEE_CHECKS", "EMBLEM_POINTS", "MARQUEE_POINTS",
		"CLASSIC_EDITORIAL_CHECKS", "CLASSIC_TICKET_CHECKS", "CLASSIC_ORBIT_CHECKS", "CLASSIC_SPLIT_CHECKS",
		"CLASSIC_EDITORIAL_POINTS", "CLASSIC_TICKET_POINTS", "CLASSIC_ORBIT_POINTS", "CLASSIC_SPLIT_POINTS",
		"SPOTLIGHT_CHECKS", "DIAGONAL_CHECKS", "GALLERY_CHECKS",
		"SPOTLIGHT_POINTS", "DIAGONAL_POINTS", "GALLERY_POINTS",
	}

	for _, template := range publishedTemplates {
		if !cardTemplates[template] {
			t.Errorf("published template %q is rejected by the service", template)
		}
	}
	if len(cardTemplates) != len(publishedTemplates) {
		t.Fatalf("service templates=%d published templates=%d", len(cardTemplates), len(publishedTemplates))
	}
}
