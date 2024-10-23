package myOidc

import (
	"context"
	"encoding/base64"
	"fmt"
	"github.com/google/uuid"
	"github.com/zitadel/oidc/v3/pkg/client/rp"
	httphelper "github.com/zitadel/oidc/v3/pkg/http"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

func RegisterLogin(mux *http.ServeMux, loginPath, callbackPath string, key []byte) {
	clientID := os.Getenv("CLIENT_ID")
	clientSecret := os.Getenv("CLIENT_SECRET")
	keyPath := os.Getenv("KEY_PATH")
	issuer := os.Getenv("ISSUER")
	port := os.Getenv("PORT")
	scopes := strings.Split(os.Getenv("SCOPES"), " ")
	responseMode := os.Getenv("RESPONSE_MODE")

	redirectURI := fmt.Sprintf("http://localhost:%v%v", port, callbackPath)
	cookieHandler := httphelper.NewCookieHandler(key, key, httphelper.WithUnsecure())

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

	// One can add a logger to the context,
	// pre-defining log attributes as required.

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

	marshalToken := func(w http.ResponseWriter, r *http.Request, tokens *oidc.Tokens[*oidc.IDTokenClaims], state string, rp rp.RelyingParty) {

		fmt.Println("tokens: ", tokens.IDToken)
		b, err := base64.StdEncoding.DecodeString(tokens.IDToken)
		if err != nil {
			log.Println("error: ", err)
		} else {
			log.Println("id token: ", string(b))
		}

		http.Redirect(w, r, "/", http.StatusFound)
	}

	mux.Handle(callbackPath, rp.CodeExchangeHandler(marshalToken, provider))
	//	mux.Handle(callbackPath, rp.CodeExchangeHandler(rp.UserinfoCallback(marshalToken), provider))

}
