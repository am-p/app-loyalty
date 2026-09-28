package service

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"clientesFrecuentes/internal/model"
	"clientesFrecuentes/internal/places"

	"github.com/google/uuid"
)

var ErrPlacesUnavailable = errors.New("places unavailable")

type ReviewPlaces interface {
	Available() bool
	Search(context.Context, string) ([]model.ReviewPlace, error)
	Resolve(context.Context, string) (string, error)
}

var placeIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,300}$`)

func validateReviewSettings(in model.ReviewSettingsInput) error {
	if in.PurchaseThreshold < 1 || in.PurchaseThreshold > 1000000 || strings.TrimSpace(in.Message) == "" || !utf8.ValidString(in.Message) || utf8.RuneCountInString(in.Message) > 500 {
		return ErrInvalidRequest
	}
	for _, r := range in.Message {
		if r == '<' || r == '>' || (unicode.IsControl(r) && r != '\t' && r != '\n' && r != '\r') {
			return ErrInvalidRequest
		}
	}
	if in.DestinationType == "MANUAL_LINK" {
		if in.GooglePlaceID != nil || (in.ManualReviewURL != nil && !places.OfficialReviewURL(*in.ManualReviewURL)) || (in.Enabled && in.ManualReviewURL == nil) {
			return ErrInvalidRequest
		}
	} else if in.DestinationType == "GOOGLE_PLACE" {
		if in.ManualReviewURL != nil || (in.GooglePlaceID != nil && !placeIDPattern.MatchString(*in.GooglePlaceID)) || (in.Enabled && in.GooglePlaceID == nil) {
			return ErrInvalidRequest
		}
	} else {
		return ErrInvalidRequest
	}
	return nil
}
func (s *Service) reviewURL(ctx context.Context, in model.ReviewSettings) (string, error) {
	if in.DestinationType == "MANUAL_LINK" && in.ManualReviewURL != nil && places.OfficialReviewURL(*in.ManualReviewURL) {
		return *in.ManualReviewURL, nil
	}
	if in.DestinationType == "GOOGLE_PLACE" && in.GooglePlaceID != nil && s.Places != nil && s.Places.Available() {
		link, e := s.Places.Resolve(ctx, *in.GooglePlaceID)
		if e == nil && places.OfficialReviewURL(link) {
			return link, nil
		}
	}
	return "", ErrPlacesUnavailable
}
func (s *Service) resolveReviewSettings(ctx context.Context, in model.ReviewSettings) model.ReviewSettings {
	in.PlacesAvailable = s.Places != nil && s.Places.Available()
	if link, e := s.reviewURL(ctx, in); e == nil {
		in.ReviewURL = &link
	}
	return in
}
func (s *Service) GetReviewSettings(ctx context.Context, a, b, id int64) (model.ReviewSettings, error) {
	out, e := s.Repo.ReviewSettings(ctx, a, b, id)
	if e != nil {
		return out, e
	}
	return s.resolveReviewSettings(ctx, out), nil
}
func (s *Service) PutReviewSettings(ctx context.Context, a, b, id int64, v int, in model.ReviewSettingsInput) (model.ReviewSettings, error) {
	if e := validateReviewSettings(in); e != nil {
		return model.ReviewSettings{}, e
	}
	out, e := s.Repo.SaveReviewSettings(ctx, a, b, id, v, in)
	if e != nil {
		return out, e
	}
	return s.resolveReviewSettings(ctx, out), nil
}
func (s *Service) SearchReviewPlaces(ctx context.Context, a, b, id int64, query string) ([]model.ReviewPlace, error) {
	branch, e := s.Repo.ReviewBranch(ctx, a, b, id)
	if e != nil {
		return nil, e
	}
	query = strings.TrimSpace(query)
	if query == "" {
		query = branch.Name
		if branch.Address != nil {
			query += " " + *branch.Address
		}
		r := []rune(query)
		if len(r) > 300 {
			query = string(r[:300])
		}
	}
	if !utf8.ValidString(query) || utf8.RuneCountInString(query) < 3 || utf8.RuneCountInString(query) > 300 {
		return nil, ErrInvalidRequest
	}
	if s.Places == nil || !s.Places.Available() {
		return nil, ErrPlacesUnavailable
	}
	out, e := s.Places.Search(ctx, query)
	if e != nil {
		return nil, ErrPlacesUnavailable
	}
	return out, nil
}
func (s *Service) PendingReviews(ctx context.Context, a int64) ([]model.ReviewInvitation, error) {
	candidates, e := s.Repo.PendingReviews(ctx, a)
	if e != nil {
		return nil, e
	}
	out := []model.ReviewInvitation{}
	// One request-wide budget prevents a queue of failing Places resolutions from serial timeouts.
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	resolved := map[string]string{}
	resolutionCount := 0
	for _, c := range candidates {
		link := ""
		var e error
		if c.Settings.DestinationType == "GOOGLE_PLACE" && c.Settings.GooglePlaceID != nil {
			id := *c.Settings.GooglePlaceID
			var cached bool
			link, cached = resolved[id]
			if !cached {
				if resolutionCount >= 10 || ctx.Err() != nil {
					continue
				}
				resolutionCount++
				link, e = s.reviewURL(ctx, c.Settings)
				resolved[id] = link
			}
			if link == "" {
				continue
			}
		} else {
			link, e = s.reviewURL(ctx, c.Settings)
		}
		if e == nil {
			c.Invitation.ReviewURL = link
			out = append(out, c.Invitation)
		}
	}
	return out, nil
}
func (s *Service) ReserveReview(ctx context.Context, a int64, id string) (model.ReviewReservation, error) {
	if _, e := uuid.Parse(id); e != nil {
		return model.ReviewReservation{}, ErrInvalidRequest
	}
	c, e := s.Repo.ReviewCandidate(ctx, a, id)
	if e != nil {
		return model.ReviewReservation{}, e
	}
	link, e := s.reviewURL(ctx, c.Settings)
	if e != nil {
		return model.ReviewReservation{}, e
	}
	c.Invitation.ReviewURL = link
	return s.Repo.ReserveReview(ctx, a, c)
}
func (s *Service) ReviewEvent(ctx context.Context, a int64, id string, in model.ReviewEventRequest) (model.ReviewEvents, error) {
	if _, e := uuid.Parse(id); e != nil {
		return model.ReviewEvents{}, ErrInvalidRequest
	}
	if _, e := uuid.Parse(in.ReservationToken); e != nil {
		return model.ReviewEvents{}, ErrInvalidRequest
	}
	if in.Event != "PRESENTADA" && in.Event != "OMITIDA" && in.Event != "CLIC" {
		return model.ReviewEvents{}, ErrInvalidRequest
	}
	return s.Repo.ReviewEvent(ctx, a, id, in)
}
