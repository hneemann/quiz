package myOidc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/hneemann/quiz/server/session"
	"github.com/zitadel/oidc/v3/pkg/client/rp"
	httphelper "github.com/zitadel/oidc/v3/pkg/http"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

func RegisterLogin(mux *http.ServeMux, loginPath, callbackPath string, cookieKey []byte, sessions *session.Sessions) {
	clientID := os.Getenv("CLIENT_ID")
	clientSecret := os.Getenv("CLIENT_SECRET")
	keyPath := os.Getenv("KEY_PATH")
	issuer := os.Getenv("ISSUER")
	tokenIdAttr := os.Getenv("ID_TOKEN_ID_ATTR")
	callbackHost := os.Getenv("HOST")
	scopes := strings.Split(os.Getenv("SCOPES"), " ")
	responseMode := os.Getenv("RESPONSE_MODE")

	redirectURI := callbackHost + callbackPath
	cookieHandler := httphelper.NewCookieHandler(cookieKey, cookieKey, httphelper.WithUnsecure())

	client := &http.Client{
		Timeout: time.Minute,
	}
	// enable outgoing request logging

	options := []rp.Option{
		rp.WithCookieHandler(cookieHandler),
		rp.WithVerifierOpts(rp.WithIssuedAtOffset(5 * time.Second)),
		rp.WithHTTPClient(client),
		rp.WithSigningAlgsFromDiscovery(),
	}
	if clientSecret == "" {
		options = append(options, rp.WithPKCE(cookieHandler))
	}
	if keyPath != "" {
		options = append(options, rp.WithJWTProfile(rp.SignerFromKeyPath(keyPath)))
	}

	provider, err := rp.NewRelyingPartyOIDC(context.Background(), issuer, clientID, clientSecret, redirectURI, scopes, options...)
	if err != nil {
		log.Fatalf("error creating provider %s", err.Error())
	}

	// generate some state (representing the state of the user in your application,
	// e.g. the page where he was before sending him to login
	state := func() string {
		return uuid.New().String()
	}

	urlOptions := []rp.URLParamOpt{
		rp.WithPromptURLParam("Welcome back!"),
	}

	if responseMode != "" {
		urlOptions = append(urlOptions, rp.WithResponseModeURLParam(oidc.ResponseMode(responseMode)))
	}

	// register the AuthURLHandler at your preferred path.
	// the AuthURLHandler creates the auth request and redirects the user to the auth server.
	// including state handling with secure cookie and the possibility to use PKCE.
	// Prompts can optionally be set to inform the server of
	// any messages that need to be prompted back to the user.
	mux.Handle(loginPath, rp.AuthURLHandler(
		state,
		provider,
		urlOptions...,
	))

	unmarshalToken := func(w http.ResponseWriter, r *http.Request, tokens *oidc.Tokens[*oidc.IDTokenClaims], state string, rp rp.RelyingParty) {
		tok := strings.Split(tokens.IDToken, ".")
		if len(tok) < 1 {
			http.Error(w, "no IDToken received", 504)
			return
		}
		b, err := base64.StdEncoding.DecodeString(tok[0])
		if err != nil {
			http.Error(w, "error decoding IDToken", 504)
			return
		}
		m := map[string]string{}
		err = json.Unmarshal(b, &m)
		if err != nil {
			http.Error(w, "error unmarshalling IDToken", 504)
			return
		}

		ident, ok := m[tokenIdAttr]
		if !ok {
			http.Error(w, "no id found in IDToken", 504)
			return
		}
		log.Println("oidc id:", ident, m)

		sessions.Create(ident, false, w)
		http.Redirect(w, r, "/", http.StatusFound)
	}

	mux.Handle(callbackPath, rp.CodeExchangeHandler(unmarshalToken, provider))
}
