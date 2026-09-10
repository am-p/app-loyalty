package service

import (
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

func (s *Service) resolveMovementIdentity(qrToken, customerCode string) (string, error) {
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

	normalizedCode := strings.ToUpper(strings.TrimPrefix(customerCode, "#"))
	if !strings.HasPrefix(normalizedCode, "USER-") {
		return "", ErrInvalidRequest
	}
	digits := strings.TrimPrefix(normalizedCode, "USER-")
	if len(digits) < 4 || len(digits) > 19 {
		return "", ErrInvalidRequest
	}
	customerID, err := strconv.ParseInt(digits, 10, 64)
	if err != nil || customerID < 1 {
		return "", ErrInvalidRequest
	}
	token, _ := s.QRForUser(customerID)
	return token, nil
}
