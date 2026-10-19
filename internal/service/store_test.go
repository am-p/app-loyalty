package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"clientesFrecuentes/internal/model"
)

func TestNormalizeStoreSearch(t *testing.T) {
	for _, tt := range []struct {
		in, want string
		bad      bool
	}{
		{"  café  ", "café", false}, {"", "", false}, {strings.Repeat("ñ", 200), strings.Repeat("ñ", 200), false},
		{strings.Repeat("ñ", 201), "", true}, {"test\x00", "", true}, {string([]byte{0xff}), "", true},
	} {
		got, err := NormalizeStoreSearch(tt.in)
		if (err != nil) != tt.bad || got != tt.want {
			t.Fatalf("got=%q err=%v", got, err)
		}
	}
}

type storeReviewPlacesFake struct {
	available bool
	url       string
	err       error
	calls     int
	deadline  time.Time
}

func (f *storeReviewPlacesFake) Available() bool { return f.available }
func (f *storeReviewPlacesFake) Search(context.Context, string) ([]model.ReviewPlace, error) {
	panic("manual review lookup must not search Places")
}
func (f *storeReviewPlacesFake) Resolve(ctx context.Context, _ string) (string, error) {
	f.calls++
	f.deadline, _ = ctx.Deadline()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	return f.url, f.err
}

func TestCustomerStoreGoogleReviewDestination(t *testing.T) {
	manual, place, unsafe := "https://g.page/r/test/review", "ChIJ_test", "https://attacker.test/review"
	for _, tt := range []struct {
		name     string
		settings model.ReviewSettingsInput
		provider *storeReviewPlacesFake
		want     *string
		bad      bool
		calls    int
	}{
		{"unconfigured manual", model.ReviewSettingsInput{DestinationType: "MANUAL_LINK"}, nil, nil, false, 0},
		{"unconfigured place", model.ReviewSettingsInput{DestinationType: "GOOGLE_PLACE"}, nil, nil, false, 0},
		{"disabled invitations manual link", model.ReviewSettingsInput{DestinationType: "MANUAL_LINK", ManualReviewURL: &manual}, nil, &manual, false, 0},
		{"unsafe manual link", model.ReviewSettingsInput{DestinationType: "MANUAL_LINK", ManualReviewURL: &unsafe}, nil, nil, true, 0},
		{"configured place unavailable", model.ReviewSettingsInput{DestinationType: "GOOGLE_PLACE", GooglePlaceID: &place}, nil, nil, true, 0},
		{"provider disabled", model.ReviewSettingsInput{DestinationType: "GOOGLE_PLACE", GooglePlaceID: &place}, &storeReviewPlacesFake{}, nil, true, 0},
		{"disabled invitations place", model.ReviewSettingsInput{DestinationType: "GOOGLE_PLACE", GooglePlaceID: &place}, &storeReviewPlacesFake{available: true, url: manual}, &manual, false, 1},
		{"provider error", model.ReviewSettingsInput{DestinationType: "GOOGLE_PLACE", GooglePlaceID: &place}, &storeReviewPlacesFake{available: true, err: errors.New("provider error")}, nil, true, 1},
		{"unsafe provider link", model.ReviewSettingsInput{DestinationType: "GOOGLE_PLACE", GooglePlaceID: &place}, &storeReviewPlacesFake{available: true, url: unsafe}, nil, true, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := &Service{}
			if tt.provider != nil {
				s.Places = tt.provider
			}
			out, err := s.customerStoreGoogleReview(t.Context(), model.ReviewSettings{BranchID: 7, ReviewSettingsInput: tt.settings})
			if tt.bad {
				if !errors.Is(err, ErrPlacesUnavailable) {
					t.Fatalf("err=%v", err)
				}
			} else if err != nil || out.BranchID != 7 || (out.ReviewURL == nil) != (tt.want == nil) || (tt.want != nil && *out.ReviewURL != *tt.want) {
				t.Fatalf("out=%+v err=%v", out, err)
			}
			if tt.provider != nil && tt.provider.calls != tt.calls {
				t.Fatalf("provider calls=%d want=%d", tt.provider.calls, tt.calls)
			}
			if tt.calls > 0 && (tt.provider.deadline.IsZero() || time.Until(tt.provider.deadline) > 5*time.Second) {
				t.Fatal("provider request lacks five-second budget")
			}
		})
	}
}

func TestCustomerStoreGoogleReviewPreservesCallerCancellation(t *testing.T) {
	place := "ChIJ_test"
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	provider := &storeReviewPlacesFake{available: true}
	s := &Service{Places: provider}
	_, err := s.customerStoreGoogleReview(ctx, model.ReviewSettings{BranchID: 1, ReviewSettingsInput: model.ReviewSettingsInput{DestinationType: "GOOGLE_PLACE", GooglePlaceID: &place}})
	if !errors.Is(err, ErrPlacesUnavailable) || provider.calls != 1 {
		t.Fatalf("err=%v calls=%d", err, provider.calls)
	}
}
