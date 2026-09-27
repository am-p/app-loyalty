package handler

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"clientesFrecuentes/internal/backofficeauth"
	"clientesFrecuentes/internal/repository"
	"clientesFrecuentes/internal/web"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

const backofficeCookie = "puntazo_backoffice"

var referralCodePattern = regexp.MustCompile(`^[A-Z0-9-]{4,40}$`)

func (h *Handler) BackofficeLogin(c *gin.Context) {
	if !h.limit(c, "backoffice-login:"+h.clientIP(c), 5, 10*time.Minute) {
		return
	}
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		TOTP     string `json:"totp"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		writeErr(c, repository.ErrInvalidRequest)
		return
	}
	u, err := h.Repo.BackofficeUserByEmail(c.Request.Context(), strings.ToLower(strings.TrimSpace(in.Email)))
	step, validTOTP := backofficeauth.MatchingTOTPStep(u.TOTPSecret, in.TOTP, time.Now())
	if err != nil || bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(in.Password)) != nil || !validTOTP {
		web.Error(c, http.StatusUnauthorized, "UNAUTHENTICATED", "Credenciales inválidas", nil)
		return
	}
	tokenBytes := make([]byte, 32)
	if _, err = rand.Read(tokenBytes); err != nil {
		writeErr(c, err)
		return
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	if err = h.Repo.CreateBackofficeSession(c.Request.Context(), u.ID, token, time.Now().Add(8*time.Hour), step); err != nil {
		if err == repository.ErrConflict {
			web.Error(c, http.StatusUnauthorized, "UNAUTHENTICATED", "Código ya utilizado", nil)
			return
		}
		writeErr(c, err)
		return
	}
	h.setBackofficeCookie(c, token, 8*3600)
	c.JSON(http.StatusOK, web.Envelope[repository.BackofficeUser]{Data: u, RequestID: web.RequestID(c)})
}

func (h *Handler) setBackofficeCookie(c *gin.Context, value string, maxAge int) {
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(backofficeCookie, value, maxAge, "/v1/backoffice", "", os.Getenv("APP_ENV") != "", true)
}

func (h *Handler) RequireBackoffice(c *gin.Context) {
	token, err := c.Cookie(backofficeCookie)
	if err != nil {
		web.Error(c, http.StatusUnauthorized, "UNAUTHENTICATED", "Sesión interna requerida", nil)
		c.Abort()
		return
	}
	u, err := h.Repo.BackofficeSession(c.Request.Context(), token)
	if err != nil {
		web.Error(c, http.StatusUnauthorized, "UNAUTHENTICATED", "Sesión interna inválida", nil)
		c.Abort()
		return
	}
	if c.Request.Method != http.MethodGet && c.GetHeader("X-Backoffice-Request") != "1" {
		web.Error(c, http.StatusForbidden, "CSRF_REQUIRED", "Solicitud no permitida", nil)
		c.Abort()
		return
	}
	c.Set("backoffice_user", u)
	c.Next()
}

func backofficeUser(c *gin.Context) repository.BackofficeUser {
	value, _ := c.Get("backoffice_user")
	return value.(repository.BackofficeUser)
}
func requireSystemAdmin(c *gin.Context) bool {
	if backofficeUser(c).Role == "ADMIN_SISTEMA" {
		return true
	}
	web.Error(c, http.StatusForbidden, "FORBIDDEN", "Requiere ADMIN_SISTEMA", nil)
	return false
}
func requireFinance(c *gin.Context) bool {
	if backofficeUser(c).Role == "FINANZAS" {
		return true
	}
	web.Error(c, http.StatusForbidden, "FORBIDDEN", "Requiere FINANZAS", nil)
	return false
}
func backofficeID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		writeErr(c, repository.ErrInvalidRequest)
		return 0, false
	}
	return id, true
}

func (h *Handler) BackofficeMe(c *gin.Context) {
	c.JSON(http.StatusOK, web.Envelope[repository.BackofficeUser]{Data: backofficeUser(c), RequestID: web.RequestID(c)})
}
func (h *Handler) BackofficeLogout(c *gin.Context) {
	token, _ := c.Cookie(backofficeCookie)
	if err := h.Repo.DeleteBackofficeSession(c.Request.Context(), token); err != nil {
		writeErr(c, err)
		return
	}
	h.setBackofficeCookie(c, "", -1)
	c.Status(http.StatusNoContent)
}

func (h *Handler) BackofficeCampaigns(c *gin.Context) {
	out, err := h.Repo.ListReferralCampaigns(c.Request.Context())
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, web.Envelope[[]repository.ReferralCampaign]{Data: out, RequestID: web.RequestID(c)})
}
func (h *Handler) BackofficeCreateCampaign(c *gin.Context) {
	if !requireSystemAdmin(c) {
		return
	}
	var in repository.ReferralCampaign
	if err := c.ShouldBindJSON(&in); err != nil {
		writeErr(c, repository.ErrInvalidRequest)
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if len(in.Name) < 3 || len(in.Name) > 120 || (in.ProgramType != "SELLOS" && in.ProgramType != "PUNTOS") || in.DiscountBPS < 0 || in.DiscountBPS > 9999 || in.RewardBPS < 0 || in.RewardBPS > 10000 || in.DiscountCharges < 0 || in.DiscountCharges > 36 || in.RewardCharges < 0 || in.RewardCharges > 36 || !in.EndsAt.After(in.StartsAt) {
		writeErr(c, repository.ErrInvalidRequest)
		return
	}
	out, err := h.Repo.CreateReferralCampaign(c.Request.Context(), in, backofficeUser(c).ID)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusCreated, web.Envelope[repository.ReferralCampaign]{Data: out, RequestID: web.RequestID(c)})
}
func (h *Handler) BackofficeCampaignActive(c *gin.Context) {
	if !requireSystemAdmin(c) {
		return
	}
	id, ok := backofficeID(c)
	if !ok {
		return
	}
	var in struct {
		Active *bool `json:"active"`
	}
	if c.ShouldBindJSON(&in) != nil || in.Active == nil {
		writeErr(c, repository.ErrInvalidRequest)
		return
	}
	if err := h.Repo.SetReferralCampaignActive(c.Request.Context(), id, backofficeUser(c).ID, *in.Active); err != nil {
		writeErr(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) BackofficeInfluencers(c *gin.Context) {
	out, err := h.Repo.ListReferralInfluencers(c.Request.Context())
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, web.Envelope[[]map[string]any]{Data: out, RequestID: web.RequestID(c)})
}
func (h *Handler) BackofficeCreateInfluencer(c *gin.Context) {
	if !requireSystemAdmin(c) {
		return
	}
	var in struct {
		Name    string `json:"name"`
		Contact string `json:"contact"`
	}
	if c.ShouldBindJSON(&in) != nil {
		writeErr(c, repository.ErrInvalidRequest)
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Contact = strings.TrimSpace(in.Contact)
	if len(in.Name) < 2 || len(in.Name) > 120 || len(in.Contact) < 3 || len(in.Contact) > 200 {
		writeErr(c, repository.ErrInvalidRequest)
		return
	}
	id, err := h.Repo.CreateReferralInfluencer(c.Request.Context(), in.Name, in.Contact, backofficeUser(c).ID)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusCreated, web.Envelope[map[string]any]{Data: map[string]any{"id": id, "name": in.Name, "contact": in.Contact}, RequestID: web.RequestID(c)})
}

func (h *Handler) BackofficeCodes(c *gin.Context) {
	out, err := h.Repo.ListReferralCodes(c.Request.Context())
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, web.Envelope[[]repository.ReferralCode]{Data: out, RequestID: web.RequestID(c)})
}
func (h *Handler) BackofficeCreateCode(c *gin.Context) {
	if !requireSystemAdmin(c) {
		return
	}
	var in struct {
		Code         string `json:"code"`
		CampaignID   int64  `json:"campaign_id"`
		InfluencerID int64  `json:"influencer_id"`
	}
	if c.ShouldBindJSON(&in) != nil {
		writeErr(c, repository.ErrInvalidRequest)
		return
	}
	in.Code = strings.ToUpper(strings.TrimSpace(in.Code))
	if !referralCodePattern.MatchString(in.Code) || in.CampaignID < 1 || in.InfluencerID < 1 {
		writeErr(c, repository.ErrInvalidRequest)
		return
	}
	out, err := h.Repo.CreateReferralInfluencerCode(c.Request.Context(), in.Code, in.CampaignID, in.InfluencerID, backofficeUser(c).ID)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusCreated, web.Envelope[repository.ReferralCode]{Data: out, RequestID: web.RequestID(c)})
}
func (h *Handler) BackofficeCodeActive(c *gin.Context) {
	if !requireSystemAdmin(c) {
		return
	}
	id, ok := backofficeID(c)
	if !ok {
		return
	}
	var in struct {
		Active *bool `json:"active"`
	}
	if c.ShouldBindJSON(&in) != nil || in.Active == nil {
		writeErr(c, repository.ErrInvalidRequest)
		return
	}
	if err := h.Repo.SetReferralCodeActive(c.Request.Context(), id, backofficeUser(c).ID, *in.Active); err != nil {
		writeErr(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
func (h *Handler) BackofficeAttributions(c *gin.Context) {
	out, err := h.Repo.ListReferralAttributions(c.Request.Context())
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, web.Envelope[[]map[string]any]{Data: out, RequestID: web.RequestID(c)})
}
func (h *Handler) BackofficeRewards(c *gin.Context) {
	out, err := h.Repo.ListReferralRewards(c.Request.Context())
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, web.Envelope[[]map[string]any]{Data: out, RequestID: web.RequestID(c)})
}

func (h *Handler) BackofficeMetrics(c *gin.Context) {
	out, err := h.Repo.ReferralMetrics(c.Request.Context())
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, web.Envelope[repository.ReferralMetrics]{Data: out, RequestID: web.RequestID(c)})
}
func (h *Handler) BackofficeSettleReward(c *gin.Context) {
	if !requireFinance(c) {
		return
	}
	invoice := strings.TrimSpace(c.Param("invoice"))
	if invoice == "" || len(invoice) > 100 {
		writeErr(c, repository.ErrInvalidRequest)
		return
	}
	if err := h.Repo.SettleReferralReward(c.Request.Context(), invoice, backofficeUser(c).ID); err != nil {
		writeErr(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
