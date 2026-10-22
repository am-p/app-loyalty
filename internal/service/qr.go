package service

import (
	"clientesFrecuentes/internal/identitycode"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
)

func (s *Service) QRForUser(userID int64) (string, []byte) {
	mac := hmac.New(sha256.New, []byte(s.Config.QRPepper))
	fmt.Fprintf(mac, "puntazo:customer:%d", userID)
	token := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	h := sha256.New()
	h.Write([]byte(s.Config.QRPepper))
	h.Write([]byte(token))
	return token, h.Sum(nil)
}

func (s *Service) QRHash(token string) []byte {
	h := sha256.New()
	h.Write([]byte(s.Config.QRPepper))
	h.Write([]byte(token))
	return h.Sum(nil)
}

func (s *Service) resolveMovementIdentity(ctx context.Context, qrToken, customerCode string) (string, error) {
	qrToken = strings.TrimSpace(qrToken)
	customerCode = strings.TrimSpace(customerCode)
	if (qrToken == "") == (customerCode == "") {
		return "", ErrInvalidRequest
	}
	if qrToken != "" {
		if len(qrToken) < 32 || len(qrToken) > 256 {
			return "", ErrInvalidRequest
		}
		return qrToken, nil
	}

	customerID, err := s.resolveCustomerCodeID(ctx, customerCode)
	if err != nil {
		return "", err
	}
	token, _ := s.QRForUser(customerID)
	return token, nil
}

func (s *Service) resolveMovementIdentityHash(ctx context.Context, qrToken, customerCode string) (string, []byte, error) {
	qrToken = strings.TrimSpace(qrToken)
	customerCode = strings.TrimSpace(customerCode)
	if (qrToken == "") == (customerCode == "") {
		return "", nil, ErrInvalidRequest
	}
	if qrToken != "" {
		if len(qrToken) < 32 || len(qrToken) > 256 {
			return "", nil, ErrInvalidRequest
		}
		return qrToken, s.QRHash(qrToken), nil
	}

	customerID, err := s.resolveCustomerCodeID(ctx, customerCode)
	if err != nil {
		return "", nil, err
	}
	storedHash, err := s.Repo.CustomerQRHashByID(ctx, customerID)
	if err != nil {
		return "", nil, err
	}
	token, _ := s.QRForUser(customerID)
	return token, storedHash, nil
}

func (s *Service) resolveCustomerCodeID(ctx context.Context, customerCode string) (int64, error) {
	customerCode = strings.TrimSpace(customerCode)
	normalizedCode := strings.ToUpper(strings.TrimPrefix(customerCode, "#"))
	if !strings.HasPrefix(normalizedCode, "USER-") {
		code, ok := identitycode.Normalize(customerCode)
		if !ok {
			return 0, ErrInvalidRequest
		}
		return s.Repo.CustomerIDByCode(ctx, code)
	}
	digits := strings.TrimPrefix(normalizedCode, "USER-")
	if len(digits) < 4 || len(digits) > 19 {
		return 0, ErrInvalidRequest
	}
	for _, r := range digits {
		if r < '0' || r > '9' {
			return 0, ErrInvalidRequest
		}
	}
	customerID, err := strconv.ParseInt(digits, 10, 64)
	if err != nil || customerID < 1 {
		return 0, ErrInvalidRequest
	}
	return customerID, nil
}
