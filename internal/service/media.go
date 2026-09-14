package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"strings"
	"time"

	"clientesFrecuentes/internal/model"

	"github.com/google/uuid"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const maxMediaBytes = 5 << 20

func (s *Service) AuthorizeBrandMedia(ctx context.Context, actorID, brandID int64) error {
	return s.Repo.AuthorizeBrandMedia(ctx, actorID, brandID)
}

func normalizeImage(input []byte, kind string) ([]byte, string, int, int, []byte, error) {
	if len(input) == 0 || len(input) > maxMediaBytes {
		return nil, "", 0, 0, nil, ErrMediaTooLarge
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(input))
	if err != nil {
		return nil, "", 0, 0, nil, ErrMediaType
	}
	if format != "jpeg" && format != "png" && format != "webp" {
		return nil, "", 0, 0, nil, ErrMediaType
	}
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 4096 || cfg.Height > 4096 || int64(cfg.Width)*int64(cfg.Height) > 16000000 {
		return nil, "", 0, 0, nil, ErrInvalidRequest
	}
	decoded, actualFormat, err := image.Decode(bytes.NewReader(input))
	if err != nil || actualFormat != format {
		return nil, "", 0, 0, nil, ErrMediaType
	}
	maxDimension := 4096
	if kind == "LOGO" {
		maxDimension = 1024
	} else if kind == "ICONO" {
		maxDimension = 512
	}
	width, height := decoded.Bounds().Dx(), decoded.Bounds().Dy()
	if width > maxDimension || height > maxDimension {
		scale := float64(maxDimension) / float64(width)
		if height > width {
			scale = float64(maxDimension) / float64(height)
		}
		targetWidth, targetHeight := int(float64(width)*scale), int(float64(height)*scale)
		if targetWidth < 1 {
			targetWidth = 1
		}
		if targetHeight < 1 {
			targetHeight = 1
		}
		resized := image.NewRGBA(image.Rect(0, 0, targetWidth, targetHeight))
		draw.CatmullRom.Scale(resized, resized.Bounds(), decoded, decoded.Bounds(), draw.Over, nil)
		decoded = resized
		width, height = targetWidth, targetHeight
	}
	var output bytes.Buffer
	mime := "image/png"
	if format == "jpeg" {
		mime = "image/jpeg"
		err = jpeg.Encode(&output, decoded, &jpeg.Options{Quality: 90})
	} else {
		encoder := png.Encoder{CompressionLevel: png.BestSpeed}
		err = encoder.Encode(&output, decoded)
	}
	if err != nil {
		return nil, "", 0, 0, nil, fmt.Errorf("normalize media: %w", err)
	}
	if output.Len() > maxMediaBytes {
		return nil, "", 0, 0, nil, ErrMediaTooLarge
	}
	sum := sha256.Sum256(output.Bytes())
	return output.Bytes(), mime, width, height, sum[:], nil
}

func (s *Service) UploadBrandImage(ctx context.Context, actorID, brandID int64, kind string, benefitID *int64, input []byte) (model.BrandImage, error) {
	if s.Media == nil {
		return model.BrandImage{}, ErrMediaUnavailable
	}
	kind = strings.ToUpper(strings.TrimSpace(kind))
	if kind != "LOGO" && kind != "ICONO" && kind != "BENEFICIO" {
		return model.BrandImage{}, ErrInvalidRequest
	}
	if (kind == "BENEFICIO") != (benefitID != nil) {
		return model.BrandImage{}, ErrInvalidRequest
	}
	// The HTTP handler authorizes before reading multipart bytes. Keep the same
	// invariant at the service boundary so non-HTTP callers cannot decode an
	// attacker-controlled image before tenant authorization.
	if err := s.AuthorizeBrandMedia(ctx, actorID, brandID); err != nil {
		return model.BrandImage{}, err
	}
	body, mime, width, height, digest, err := normalizeImage(input, kind)
	if err != nil {
		return model.BrandImage{}, err
	}
	id := uuid.NewString()
	ext := "png"
	if mime == "image/jpeg" {
		ext = "jpg"
	}
	item := model.BrandImage{ID: id, BrandID: brandID, Type: kind, BenefitID: benefitID, ObjectKey: fmt.Sprintf("brands/%d/%s.%s", brandID, id, ext), MIMEType: mime, ByteSize: int64(len(body)), Width: width, Height: height}
	item, err = s.Repo.ReserveBrandImage(ctx, actorID, brandID, item, digest)
	if err != nil {
		return item, err
	}
	compensationCtx, cancelCompensation := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancelCompensation()
	if err = s.Media.Put(ctx, item.ObjectKey, mime, body, digest); err != nil {
		s.Repo.FailBrandImage(compensationCtx, id, err.Error())
		return model.BrandImage{}, ErrMediaUnavailable
	}
	objectKey := item.ObjectKey
	item, err = s.Repo.ActivateBrandImage(ctx, actorID, id)
	if err != nil {
		_ = s.Media.Delete(compensationCtx, objectKey)
		s.Repo.FailBrandImage(compensationCtx, id, err.Error())
		return model.BrandImage{}, err
	}
	signed, signErr := s.signBrandImage(ctx, item)
	if signErr != nil {
		return item, nil
	}
	return signed, nil
}

func (s *Service) BrandImages(ctx context.Context, actorID, brandID int64) ([]model.BrandImage, error) {
	if s.Media == nil {
		return nil, ErrMediaUnavailable
	}
	items, err := s.Repo.ListBrandImages(ctx, actorID, brandID)
	if err != nil {
		return nil, err
	}
	for i := range items {
		items[i], err = s.signBrandImage(ctx, items[i])
		if err != nil {
			return nil, err
		}
	}
	return items, nil
}
func (s *Service) signBrandImage(ctx context.Context, item model.BrandImage) (model.BrandImage, error) {
	url, err := s.Media.SignedGet(ctx, item.ObjectKey, s.Config.MediaURLTTL)
	if err != nil {
		return item, ErrMediaUnavailable
	}
	expires := s.Now().Add(s.Config.MediaURLTTL)
	item.URL = url
	item.URLExpiresAt = &expires
	return item, nil
}
func (s *Service) DeleteBrandImage(ctx context.Context, actorID, brandID int64, id string, version int) error {
	if _, err := uuid.Parse(id); err != nil {
		return ErrInvalidRequest
	}
	return s.Repo.DeleteBrandImage(ctx, actorID, brandID, id, version)
}
