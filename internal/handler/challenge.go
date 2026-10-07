package handler

import (
	"clientesFrecuentes/internal/challenge"
	"clientesFrecuentes/internal/web"
	"crypto/rand"
	"encoding/base64"
	"github.com/gin-gonic/gin"
	"html/template"
	"net/http"
	"net/url"
	"strings"
)

var challengePage = template.Must(template.New("challenge").Parse(`<!doctype html><html lang="es"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Puntazo · Verificación</title><style nonce="{{.Nonce}}">body{font:17px system-ui;margin:40px auto;padding:20px;max-width:460px;color:#222}h1{font-size:26px}</style><h1>Verificá que sos una persona</h1><p>Al completar el desafío volverás a Puntazo.</p><div class="cf-turnstile" data-sitekey="{{.SiteKey}}" data-action="{{.Flow}}" data-cdata="{{.State}}" data-callback="completed"></div><p id="result" role="status"></p><script nonce="{{.Nonce}}">const flow={{.Flow}},state={{.State}},platform={{.Platform}},origin={{.AppOrigin}};async function completed(token){try{let response=await fetch("challenge/verify",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({token,flow,state})});let body=await response.json();if(!response.ok)throw Error("No se pudo validar el desafío. Abrí uno nuevo.");const data={type:"puntazo-captcha",receipt:body.data.receipt,state};history.replaceState(null,"",location.pathname);if(platform==="native"){location.href="puntazo://auth/captcha?receipt="+encodeURIComponent(data.receipt)+"&state="+encodeURIComponent(state)}else{if(window.opener)window.opener.postMessage(data,origin);if(window.parent!==window)window.parent.postMessage(data,origin);document.getElementById("result").textContent="Verificación completada. Podés volver a Puntazo."}}catch(error){document.getElementById("result").textContent=error.message}}</script><script src="https://challenges.cloudflare.com/turnstile/v0/api.js" async defer></script></html>`))

func (h *Handler) Challenge(c *gin.Context) {
	if h.Service == nil || h.Service.Challenge == nil || !h.Service.Config.CaptchaEnabled {
		writeErr(c, challenge.ErrUnavailable)
		return
	}
	if !h.limit(c, "captcha-challenge:ip:"+h.clientIP(c), 10, loginWindow) {
		return
	}
	flow, state := c.Query("flow"), c.Query("state")
	platform := c.DefaultQuery("platform", "native")
	if platform != "native" && platform != "web" {
		writeErr(c, challenge.ErrInvalid)
		return
	}
	if err := h.Service.Challenge.Begin(c.Request.Context(), flow, state); err != nil {
		writeErr(c, err)
		return
	}
	bytes := make([]byte, 18)
	if _, err := rand.Read(bytes); err != nil {
		writeErr(c, challenge.ErrUnavailable)
		return
	}
	nonce := base64.RawURLEncoding.EncodeToString(bytes)
	appURL, _ := url.Parse(h.Service.Config.PublicAppURL)
	origin := ""
	if appURL != nil {
		origin = appURL.Scheme + "://" + appURL.Host
	}
	c.Header("Content-Security-Policy", "default-src 'none'; script-src 'nonce-"+nonce+"' https://challenges.cloudflare.com; style-src 'nonce-"+nonce+"'; frame-src https://challenges.cloudflare.com; connect-src 'self' https://challenges.cloudflare.com; base-uri 'none'; form-action 'none'")
	c.Header("Referrer-Policy", "no-referrer")
	c.Header("Cache-Control", "no-store, private")
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Status(http.StatusOK)
	_ = challengePage.Execute(c.Writer, map[string]string{"Nonce": nonce, "Flow": flow, "State": state, "Platform": platform, "SiteKey": h.Service.Challenge.SiteKey, "AppOrigin": origin})
}
func (h *Handler) VerifyChallenge(c *gin.Context) {
	if h.Service == nil || h.Service.Challenge == nil || !h.Service.Config.CaptchaEnabled {
		writeErr(c, challenge.ErrUnavailable)
		return
	}
	if !h.limit(c, "captcha-verify:ip:"+h.clientIP(c), 10, loginWindow) {
		return
	}
	var req struct {
		Token string `json:"token"`
		Flow  string `json:"flow"`
		State string `json:"state"`
	}
	if decode(c, &req) != nil {
		writeErr(c, challenge.ErrInvalid)
		return
	}
	receipt, err := h.Service.Challenge.Verify(c.Request.Context(), strings.TrimSpace(req.Token), req.Flow, req.State, h.clientIP(c))
	if err != nil {
		writeErr(c, err)
		return
	}
	c.Header("Cache-Control", "no-store, private")
	c.JSON(http.StatusOK, web.Envelope[map[string]any]{Data: map[string]any{"receipt": receipt, "expires_in": 120, "state": req.State}, RequestID: web.RequestID(c)})
}
