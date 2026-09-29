package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"clientesFrecuentes/internal/model"
)

func TestReviewSettingsValidation(t *testing.T) {
	manual := "https://g.page/r/a/review"
	place := "ChIJ_123"
	valid := model.ReviewSettingsInput{PurchaseThreshold: 1, Message: "¿Qué tal?", DestinationType: "MANUAL_LINK"}
	if e := validateReviewSettings(valid); e != nil {
		t.Fatal(e)
	}
	valid.Enabled = true
	valid.ManualReviewURL = &manual
	if e := validateReviewSettings(valid); e != nil {
		t.Fatal(e)
	}
	cases := []model.ReviewSettingsInput{valid, valid, valid, valid, valid, valid, valid, valid, valid, valid}
	cases[0].PurchaseThreshold = 0
	cases[1].PurchaseThreshold = 1000001
	cases[2].Message = strings.Repeat("ñ", 501)
	cases[3].Message = "<b>Hola</b>"
	cases[4].Message = "hola\x00"
	cases[5].Message = " "
	cases[6].GooglePlaceID = &place
	cases[7].ManualReviewURL = nil
	cases[8].DestinationType = "OTHER"
	bad := "https://attacker.test/review"
	cases[9].ManualReviewURL = &bad
	for i, in := range cases {
		if !errors.Is(validateReviewSettings(in), ErrInvalidRequest) {
			t.Errorf("invalid case %d accepted", i)
		}
	}
	valid.Message = "Hola\nContanos tu experiencia\t\r"
	if e := validateReviewSettings(valid); e != nil {
		t.Fatal("plain text line breaks", e)
	}
	valid.Message = strings.Repeat("ñ", 500)
	if e := validateReviewSettings(valid); e != nil {
		t.Fatal("Unicode length", e)
	}
	valid.DestinationType = "GOOGLE_PLACE"
	valid.ManualReviewURL = nil
	valid.GooglePlaceID = &place
	if e := validateReviewSettings(valid); e != nil {
		t.Fatal(e)
	}
	bad = "../places/escape"
	valid.GooglePlaceID = &bad
	if !errors.Is(validateReviewSettings(valid), ErrInvalidRequest) {
		t.Fatal("place path accepted")
	}
}
func TestManualDestinationIndependentOfPlaces(t *testing.T) {
	url := "https://g.page/r/a/review"
	s := &Service{}
	out := s.resolveReviewSettings(context.Background(), model.ReviewSettings{ReviewSettingsInput: model.ReviewSettingsInput{DestinationType: "MANUAL_LINK", ManualReviewURL: &url}})
	if out.PlacesAvailable || out.ReviewURL == nil || *out.ReviewURL != url {
		t.Fatalf("manual=%+v", out)
	}
	out = s.resolveReviewSettings(context.Background(), model.ReviewSettings{ReviewSettingsInput: model.ReviewSettingsInput{DestinationType: "GOOGLE_PLACE", GooglePlaceID: &url}})
	if out.ReviewURL != nil {
		t.Fatal("unavailable config lost editable boundary")
	}
}
